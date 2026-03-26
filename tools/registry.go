package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"okr-agent/llm"
)

// Registry 管理工具的注册和分发。
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 创建一个空的工具注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 向注册表中添加一个工具。
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// GetToolParams 将所有已注册的工具转换为 Azure OpenAI 工具格式。
func (r *Registry) GetToolParams() []llm.Tool {
	params := make([]llm.Tool, 0, len(r.tools))
	for _, t := range r.tools {
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
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, input)
}
