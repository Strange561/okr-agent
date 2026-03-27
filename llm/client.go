package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// defaultAPIVersion 是 Azure OpenAI API 的版本号。
// 使用 "2025-04-01-preview" 版本，该版本支持 function calling（工具调用）功能。
// Azure OpenAI 的 API 版本通过 URL 查询参数传递，不同版本支持的功能不同。
// 如果要升级 API 版本，需要确认新版本兼容现有的请求/响应格式。
const defaultAPIVersion = "2025-04-01-preview"

// Client 是 Azure OpenAI 聊天补全 API 的 HTTP 客户端。
//
// 与直接使用 OpenAI API 不同，Azure OpenAI 有自己的认证方式和 URL 格式：
//   - 使用 "api-key" 请求头进行认证（而非 Bearer Token）
//   - URL 中包含 deployment 名称（对应特定的模型部署）
//   - API 版本通过查询参数指定
//
// Client 是线程安全的，可以在多个 goroutine 中共享使用。
type Client struct {
	endpoint   string       // Azure OpenAI 端点 URL，例如 https://xxx.openai.azure.com
	apiKey     string       // Azure OpenAI API 密钥，用于请求认证
	deployment string       // 模型部署名称，对应 Azure 控制台中创建的部署
	apiVersion string       // API 版本字符串，不同版本支持不同的功能特性
	http       *http.Client // 底层 HTTP 客户端，用于发送请求
}

// NewClient 创建一个新的 Azure OpenAI 客户端。
//
// 参数说明：
//   - endpoint: Azure OpenAI 资源的端点 URL（在 Azure Portal 中获取）
//   - apiKey: API 访问密钥（在 Azure Portal 的"密钥和端点"中获取）
//   - deployment: 模型部署名称（在 Azure OpenAI Studio 中创建的部署）
//
// 该客户端使用默认的 http.Client，在生产环境中可考虑设置超时和连接池。
func NewClient(endpoint, apiKey, deployment string) *Client {
	return &Client{
		endpoint:   endpoint,
		apiKey:     apiKey,
		deployment: deployment,
		apiVersion: defaultAPIVersion,
		http:       &http.Client{},
	}
}

// Deployment 返回配置的部署名称。
// 主要用于日志输出，方便确认当前使用的是哪个模型部署。
func (c *Client) Deployment() string {
	return c.deployment
}

// CreateMessage 向 Azure OpenAI 发送聊天补全请求。
//
// 这是整个 Agent 系统中最核心的方法——每次 Agent 需要 LLM 做决策时都会调用此方法。
// 工作流程：
//  1. 将请求结构体序列化为 JSON
//  2. 构建 Azure OpenAI 格式的 URL（包含 deployment 和 api-version）
//  3. 发送 HTTP POST 请求，带上 api-key 认证头
//  4. 解析响应，处理各种错误情况
//  5. 返回包含 LLM 回复的 Response 对象
//
// 该方法支持 context 取消，当上游请求超时或被取消时会自动终止。
func (c *Client) CreateMessage(ctx context.Context, req Request) (*Response, error) {
	// 第一步：序列化请求体为 JSON 格式
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 第二步：构建 Azure OpenAI 专用的 API URL
	// 格式: {endpoint}/openai/deployments/{deployment}/chat/completions?api-version={version}
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		c.endpoint, c.deployment, c.apiVersion)

	// 第三步：创建带有 context 的 HTTP 请求，支持超时和取消
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", c.apiKey) // Azure OpenAI 使用 api-key 头认证（不同于标准 OpenAI 的 Bearer Token）

	// 第四步：发送 HTTP 请求
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	// 第五步：读取完整的响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// 第六步：反序列化 JSON 响应
	var chatResp Response
	if err := json.Unmarshal(body, &chatResp); err != nil {
		// 如果 JSON 解析失败，将原始响应体包含在错误信息中，便于调试
		return nil, fmt.Errorf("unmarshal response: %w (body: %s)", err, string(body))
	}

	// 第七步：检查 API 层面的错误（如模型不存在、配额超限等）
	if chatResp.Error != nil {
		return nil, fmt.Errorf("azure openai error: %s", chatResp.Error.Message)
	}

	// 第八步：检查 HTTP 状态码（正常应为 200）
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure openai error (%d): %s", resp.StatusCode, string(body))
	}

	// 第九步：确保响应中包含至少一个 Choice
	// 正常情况下 Choices 数组应至少有一个元素
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &chatResp, nil
}
