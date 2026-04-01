// Package llm 封装了与 LLM（大语言模型）API 交互所需的类型定义。
//
// 本包定义了 Chat Completions API 的请求/响应数据结构，
// 包括消息（Message）、工具调用（ToolCall）、工具定义（Tool）等。
// 这些类型是 Agent 与 LLM 通信的基础数据协议。
//
// 在整体架构中，llm 包处于最底层，不依赖其他业务包，
// 仅提供纯粹的数据结构定义和 HTTP 客户端功能。
package llm

import "encoding/json"

// Message 表示 OpenAI 格式的聊天消息。
//
// 在 Agent 架构中，消息是 LLM 交互的基本单元。每条消息都有一个角色（role），
// 用于区分消息的来源和用途：
//   - "system"：系统提示词，定义 Agent 的行为和能力
//   - "user"：用户的输入消息
//   - "assistant"：LLM 的回复，可能包含文本或工具调用
//   - "tool"：工具执行结果的返回消息
//
// ToolCalls 和 ToolCallID 用于支持 OpenAI 的 function calling 机制，
// 这是 Agent 能够调用外部工具（如查询 OKR、发送消息）的关键。
type Message struct {
	Role       string     `json:"role"`                  // 消息角色：system / user / assistant / tool
	Content    string     `json:"content"`               // 消息文本内容
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // 助手请求的工具调用列表（仅 assistant 角色使用）
	ToolCallID string     `json:"tool_call_id,omitempty"` // 工具调用结果对应的调用 ID（仅 tool 角色使用）
}

// ToolCall 表示助手发起的工具调用。
//
// 当 LLM 判断需要调用外部工具时，会在响应中返回 ToolCall 列表。
// 每个 ToolCall 包含一个唯一 ID（用于将工具结果关联回对应的调用）
// 和具体的函数调用信息。Agent 的 ReAct 循环会解析这些调用，
// 执行相应的工具，然后将结果以 tool 角色消息的形式反馈给 LLM。
type ToolCall struct {
	ID       string       `json:"id"`       // 工具调用的唯一标识符，由 API 生成
	Type     string       `json:"type"`     // 调用类型，目前固定为 "function"
	Function FunctionCall `json:"function"` // 具体的函数名和参数
}

// FunctionCall 包含函数名称和参数 JSON 字符串。
//
// Arguments 是 JSON 格式的字符串（而非结构化对象），因为不同工具的参数结构各不相同，
// 需要在具体的工具实现中进行反序列化。这种设计提供了最大的灵活性。
type FunctionCall struct {
	Name      string `json:"name"`      // 要调用的函数名称，对应 Tool 注册时的名称
	Arguments string `json:"arguments"` // 函数参数的 JSON 字符串，需在工具内部解析
}

// Tool 定义 OpenAI 格式的工具。
//
// 工具定义会在每次 API 请求中发送给 LLM，让模型知道有哪些工具可用。
// LLM 会根据用户的请求和工具的描述，自主决定是否调用某个工具。
// 这是 Agent 架构中"感知-思考-行动"循环的核心：LLM 既是决策者也是协调者。
type Tool struct {
	Type     string       `json:"type"`     // 工具类型，目前固定为 "function"
	Function ToolFunction `json:"function"` // 函数的详细定义
}

// ToolFunction 保存函数定义，包括名称、描述和参数模式。
//
// Description 字段非常重要——LLM 主要依赖描述文本来理解工具的用途和使用场景。
// 好的工具描述能显著提高 Agent 选择正确工具的准确率。
// Parameters 使用 json.RawMessage 类型以支持灵活的 JSON Schema 定义。
type ToolFunction struct {
	Name        string          `json:"name"`        // 函数名称，Agent 通过此名称调用工具
	Description string          `json:"description"` // 函数的自然语言描述，帮助 LLM 理解何时使用
	Parameters  json.RawMessage `json:"parameters"`  // JSON Schema 格式的参数定义
}

// Request 是 LLM 聊天补全请求。
//
// 每次 Agent 循环都会构建一个新的 Request，包含：
//   - Model：模型名称（如 kimi-k2.5）
//   - Messages：完整的对话历史（system + user + assistant + tool 消息）
//   - Tools：当前可用的工具列表
//   - MaxCompletionTokens：限制回复长度，避免生成过长的内容
//
// 注意：Messages 的顺序很重要，system 消息必须在最前面。
type Request struct {
	Model               string    `json:"model,omitempty"`                   // 模型名称
	Messages            []Message `json:"messages"`                          // 对话消息列表，按时间顺序排列
	Tools               []Tool    `json:"tools,omitempty"`                   // 可用工具列表，LLM 据此决定是否调用工具
	MaxCompletionTokens int       `json:"max_completion_tokens,omitempty"`   // 最大生成 token 数，防止回复过长
}

// Response 是 LLM 聊天补全响应。
//
// API 返回的响应中最重要的是 Choices 数组。通常我们只使用第一个 Choice。
// Usage 字段用于监控 token 消耗，帮助控制成本。
// Error 字段在 API 层面出错时会被填充（如配额超限、模型不可用等）。
type Response struct {
	ID      string    `json:"id"`              // 响应的唯一标识
	Choices []Choice  `json:"choices"`         // 补全选项列表，通常只有一个
	Usage   Usage     `json:"usage"`           // Token 用量统计
	Error   *APIError `json:"error,omitempty"` // API 错误信息（正常时为 nil）
}

// Choice 表示单个补全选项。
//
// FinishReason 是 Agent 循环的关键判断依据：
//   - "stop"：LLM 已生成完整回复，Agent 循环结束
//   - "tool_calls"：LLM 请求调用工具，Agent 需要执行工具并继续循环
//   - "length"：达到 token 上限被截断，通常也结束循环
type Choice struct {
	Message      Message `json:"message"`       // LLM 生成的消息
	FinishReason string  `json:"finish_reason"` // 结束原因：stop / tool_calls / length
}

// Usage 包含 token 用量信息。
//
// 在生产环境中监控 token 用量非常重要，因为 LLM API 按 token 计费。
// Agent 循环中可能有多轮 LLM 调用（每次工具调用后都会再次请求 LLM），
// 所以单次用户请求的总 token 消耗可能是单次 API 调用的数倍。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`     // 输入（提示词）消耗的 token 数
	CompletionTokens int `json:"completion_tokens"` // 输出（生成内容）消耗的 token 数
	TotalTokens      int `json:"total_tokens"`      // 总 token 数 = PromptTokens + CompletionTokens
}

// APIError 在 API 返回错误时使用。
//
// 常见的错误类型包括：
//   - 认证失败（无效的 API Key）
//   - 配额超限（rate limit exceeded）
//   - 模型不可用（deployment not found）
//   - 内容过滤（content filter triggered）
type APIError struct {
	Message string `json:"message"` // 错误描述信息
	Type    string `json:"type"`    // 错误类型分类
	Code    string `json:"code"`    // 错误代码，用于程序化处理
}
