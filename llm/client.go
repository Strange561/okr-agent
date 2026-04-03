package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client 是 LLM 聊天补全 API 的 HTTP 客户端。
//
// 支持兼容 OpenAI 格式的 API（如 Kimi），使用 Bearer Token 认证。
// Client 是线程安全的，可以在多个 goroutine 中共享使用。
type Client struct {
	endpoint string       // API 端点 URL，例如 https://api.moonshot.cn/v1
	apiKey   string       // API 密钥，用于 Bearer Token 认证
	model    string       // 模型名称，例如 kimi-k2.5
	http     *http.Client // 底层 HTTP 客户端
}

// NewClient 创建一个新的 LLM 客户端。
//
// 参数说明：
//   - endpoint: API 端点 URL
//   - apiKey: API 访问密钥
//   - model: 模型名称
func NewClient(endpoint, apiKey, model string) *Client {
	return &Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    model,
		http:     &http.Client{Timeout: 120 * time.Second},
	}
}

// Model 返回配置的模型名称。
func (c *Client) Model() string {
	return c.model
}

// CreateMessage 向 LLM 发送聊天补全请求。
//
// 这是整个 Agent 系统中最核心的方法——每次 Agent 需要 LLM 做决策时都会调用此方法。
// 工作流程：
//  1. 自动填充模型名称到请求中
//  2. 将请求结构体序列化为 JSON
//  3. 构建 API URL 并发送 HTTP POST 请求
//  4. 解析响应，处理各种错误情况
//  5. 返回包含 LLM 回复的 Response 对象
func (c *Client) CreateMessage(ctx context.Context, req Request) (*Response, error) {
	// 自动填充模型名称
	req.Model = c.model

	// 序列化请求体为 JSON 格式
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 构建 API URL
	url := fmt.Sprintf("%s/chat/completions", c.endpoint)

	// 创建带有 context 的 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	// 发送 HTTP 请求
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	// 读取完整的响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// 反序列化 JSON 响应
	var chatResp Response
	if err := json.Unmarshal(body, &chatResp); err != nil {
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500] + "...(truncated)"
		}
		return nil, fmt.Errorf("unmarshal response: %w (body: %s)", err, preview)
	}

	// 检查 API 层面的错误
	if chatResp.Error != nil {
		return nil, fmt.Errorf("LLM API error: %s", chatResp.Error.Message)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API error (%d): %s", resp.StatusCode, string(body))
	}

	// 确保响应中包含至少一个 Choice
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &chatResp, nil
}
