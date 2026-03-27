package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"okr-agent/llm"
)

// Registry 管理工具的注册和分发。
//
// Registry 是 Agent 工具体系的中枢。它充当工具的"电话簿"：
//   - 在启动阶段，所有工具通过 Register 方法注册到 Registry 中
//   - 在 Agent 循环中，GetToolParams 将注册的工具转换为 LLM 可理解的格式
//   - 当 LLM 决定调用某个工具时，Execute 根据名称找到对应的工具并执行
//
// 设计简洁，使用 map[string]Tool 存储工具，通过名称进行 O(1) 查找。
// 在生产环境中可以考虑添加中间件（如日志、超时、权限检查等）。
type Registry struct {
	tools map[string]Tool // 工具映射表，key 为工具名称，value 为工具实例
}

// NewRegistry 创建一个空的工具注册表。
// 创建后需要通过 Register 方法逐个添加工具。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 向注册表中添加一个工具。
//
// 使用工具的 Name() 作为 key 存储到内部 map 中。
// 如果注册同名工具，后注册的会覆盖先注册的。
// 通常在 main 函数初始化阶段调用此方法。
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// GetToolParams 将所有已注册的工具转换为 Azure OpenAI 工具格式。
//
// 此方法在每次 Agent 循环的 LLM 请求中被调用，
// 将注册表中的工具信息打包为 LLM 可理解的 JSON 格式。
// LLM 通过这些信息来决定是否需要调用工具，以及调用哪个工具。
//
// 返回的 []llm.Tool 会被放入 Request.Tools 字段中。
func (r *Registry) GetToolParams() []llm.Tool {
	params := make([]llm.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		// 将每个 Tool 接口实例转换为 llm.Tool 结构体
		params = append(params, llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.InputSchema(),
			},
		})
	}
	return params
}

// Execute 根据名称分发工具调用。
//
// 当 LLM 返回 tool_calls 时，Agent 循环会调用此方法执行具体的工具。
// 工作流程：
//  1. 根据名称在 map 中查找对应的工具
//  2. 如果找不到，返回错误（通常说明 LLM "幻觉"了一个不存在的工具名）
//  3. 调用工具的 Execute 方法执行实际操作
//
// 参数：
//   - name: 工具名称，由 LLM 在 tool_calls 中指定
//   - input: 工具的输入参数（JSON 格式），由 LLM 生成
//
// 返回的字符串结果会作为 tool 角色的消息内容反馈给 LLM。
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, input)
}
