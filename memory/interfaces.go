package memory

import (
	"context"

	"okr-agent/llm"
)

// ConversationStore 定义对话持久化能力，用于解耦和测试。
type ConversationStore interface {
	GetConversation(ctx context.Context, userID string) ([]llm.Message, error)
	SaveConversation(ctx context.Context, userID string, messages []llm.Message) error
}

// SnapshotStore 定义 OKR 快照存储能力。
type SnapshotStore interface {
	SaveOKRSnapshot(ctx context.Context, userID, month, okrData string) error
	GetOKRSnapshots(ctx context.Context, userID string, limit int) ([]OKRSnapshot, error)
}

// UserContextStore 定义用户上下文存储能力。
type UserContextStore interface {
	GetUserContext(ctx context.Context, userID string) (*UserContext, error)
	TouchUserInteraction(ctx context.Context, userID string) error
}

// SchedulerStateStore 定义调度状态存储能力。
type SchedulerStateStore interface {
	GetSchedulerState(ctx context.Context, userID string) (*SchedulerState, error)
	SaveSchedulerState(ctx context.Context, ss *SchedulerState) error
	UpdateReminderTime(ctx context.Context, userID string) error
}
