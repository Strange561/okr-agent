package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"okr-agent/llm"
)

const conversationTTL = 24 * time.Hour

// SaveConversation 持久化指定用户的对话记录。
func (s *Store) SaveConversation(ctx context.Context, userID string, messages []llm.Message) error {
	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO conversations (user_id, messages, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET messages = excluded.messages, updated_at = CURRENT_TIMESTAMP`,
		userID, string(data))
	if err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return nil
}

// GetConversation 加载用户的对话记录。如果已过期或未找到则返回 nil。
func (s *Store) GetConversation(ctx context.Context, userID string) ([]llm.Message, error) {
	var messagesJSON string
	var updatedAt time.Time

	err := s.db.QueryRowContext(ctx,
		`SELECT messages, updated_at FROM conversations WHERE user_id = ?`, userID,
	).Scan(&messagesJSON, &updatedAt)

	if err != nil {
		return nil, nil // 未找到 → 新对话
	}

	if time.Since(updatedAt) > conversationTTL {
		// 已过期 — 删除并返回 nil
		s.db.ExecContext(ctx, `DELETE FROM conversations WHERE user_id = ?`, userID)
		return nil, nil
	}

	var messages []llm.Message
	if err := json.Unmarshal([]byte(messagesJSON), &messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return messages, nil
}

// DeleteConversation 删除一条对话记录。
func (s *Store) DeleteConversation(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE user_id = ?`, userID)
	return err
}
