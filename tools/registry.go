package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"okr-agent/claude"
)

// Registry manages tool registration and dispatch.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// GetToolParams converts all registered tools to Azure OpenAI tool format.
func (r *Registry) GetToolParams() []claude.Tool {
	params := make([]claude.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		params = append(params, claude.Tool{
			Type: "function",
			Function: claude.ToolFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.InputSchema(),
			},
		})
	}
	return params
}

// Execute dispatches a tool call by name.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(ctx, input)
}
