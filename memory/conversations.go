package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"okr-agent/claude"
)

const conversationTTL = 24 * time.Hour

// SaveConversation persists a conversation for a given user.
func (s *Store) SaveConversation(ctx context.Context, userID string, messages []claude.Message) error {
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

// GetConversation loads the conversation for a user. Returns nil if expired or not found.
func (s *Store) GetConversation(ctx context.Context, userID string) ([]claude.Message, error) {
	var messagesJSON string
	var updatedAt time.Time

	err := s.db.QueryRowContext(ctx,
		`SELECT messages, updated_at FROM conversations WHERE user_id = ?`, userID,
	).Scan(&messagesJSON, &updatedAt)

	if err != nil {
		return nil, nil // not found → new conversation
	}

	if time.Since(updatedAt) > conversationTTL {
		// Expired — delete and return nil
		s.db.ExecContext(ctx, `DELETE FROM conversations WHERE user_id = ?`, userID)
		return nil, nil
	}

	var messages []claude.Message
	if err := json.Unmarshal([]byte(messagesJSON), &messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return messages, nil
}

// DeleteConversation removes a conversation.
func (s *Store) DeleteConversation(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE user_id = ?`, userID)
	return err
}
