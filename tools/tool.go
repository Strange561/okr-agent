package tools

import (
	"context"
	"encoding/json"
)

// Tool defines the interface for agent tools.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}
