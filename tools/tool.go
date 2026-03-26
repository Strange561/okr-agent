package tools

import (
	"context"
	"encoding/json"
)

// Tool 定义 Agent 工具的接口。
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}
