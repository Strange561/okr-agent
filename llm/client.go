package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultAPIVersion = "2025-04-01-preview"

// Client 是 Azure OpenAI 聊天补全 API 的 HTTP 客户端。
type Client struct {
	endpoint   string // 例如 https://xxx.openai.azure.com
	apiKey     string
	deployment string
	apiVersion string
	http       *http.Client
}

// NewClient 创建一个新的 Azure OpenAI 客户端。
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
func (c *Client) Deployment() string {
	return c.deployment
}

// CreateMessage 向 Azure OpenAI 发送聊天补全请求。
func (c *Client) CreateMessage(ctx context.Context, req Request) (*Response, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		c.endpoint, c.deployment, c.apiVersion)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var chatResp Response
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (body: %s)", err, string(body))
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("azure openai error: %s", chatResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure openai error (%d): %s", resp.StatusCode, string(body))
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &chatResp, nil
}
