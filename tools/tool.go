// Package tools 定义了 Agent 可调用的工具（Tools）体系。
//
// 在 Agent 架构中，工具是 Agent 与外部世界交互的"手臂"。
// LLM 本身只能生成文本，通过工具调用（function calling），
// Agent 可以执行实际操作，如查询飞书 OKR 数据、发送消息等。
//
// 本包提供了：
//   - Tool 接口定义：所有工具必须实现的标准接口
//   - Registry：工具注册表，管理工具的注册和分发
//   - 各种具体工具实现：OKR 查询、消息发送、团队管理等
//
// 工具设计遵循 OpenAI Function Calling 规范：
// 每个工具有名称（Name）、描述（Description）、参数模式（InputSchema）和执行方法（Execute）。
// LLM 通过名称和描述来决定何时使用哪个工具。
package tools

import (
	"context"
	"encoding/json"
)

// Tool 定义 Agent 工具的接口。
//
// 这是工具体系的核心抽象。任何需要被 Agent 调用的外部能力
// 都应实现此接口。接口设计遵循 OpenAI Function Calling 规范：
//
//   - Name() 返回工具名称，LLM 通过此名称引用工具
//   - Description() 返回工具描述，帮助 LLM 理解何时使用此工具
//   - InputSchema() 返回 JSON Schema，定义工具接受的参数格式
//   - Execute() 执行实际操作并返回结果字符串
//
// 实现 Tool 接口时的注意事项：
//   - Name 应简洁明了，使用 snake_case 格式（如 "get_user_okrs"）
//   - Description 应清晰说明工具的功能和使用场景
//   - InputSchema 应完整定义所有必需和可选参数
//   - Execute 应返回人类和 LLM 都能理解的结果文本
type Tool interface {
	Name() string                                                    // 返回工具名称（唯一标识符）
	Description() string                                             // 返回工具的自然语言描述
	InputSchema() json.RawMessage                                    // 返回 JSON Schema 格式的参数定义
	Execute(ctx context.Context, input json.RawMessage) (string, error) // 执行工具并返回结果
}
