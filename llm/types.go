package llm

import "encoding/json"

// Message 表示 OpenAI 格式的聊天消息。
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall 表示助手发起的工具调用。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function" 类型
	Function FunctionCall `json:"function"`
}

// FunctionCall 包含函数名称和参数 JSON 字符串。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool 定义 OpenAI 格式的工具。
type Tool struct {
	Type     string       `json:"type"` // "function" 类型
	Function ToolFunction `json:"function"`
}

// ToolFunction 保存函数定义。
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request 是 Azure OpenAI 聊天补全请求。
type Request struct {
	Messages            []Message `json:"messages"`
	Tools               []Tool    `json:"tools,omitempty"`
	MaxCompletionTokens int       `json:"max_completion_tokens,omitempty"`
}

// Response 是 Azure OpenAI 聊天补全响应。
type Response struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
	Error   *APIError `json:"error,omitempty"`
}

// Choice 表示单个补全选项。
type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage 包含 token 用量信息。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// APIError 在 API 返回错误时使用。
type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}
