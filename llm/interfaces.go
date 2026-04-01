package llm

import "context"

// ChatClient 定义 LLM 聊天补全能力，用于解耦和测试。
type ChatClient interface {
	CreateMessage(ctx context.Context, req Request) (*Response, error)
	Model() string
}
