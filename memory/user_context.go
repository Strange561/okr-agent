package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UserContext 保存用户偏好和 Agent 维护的备注信息。
type UserContext struct {
	UserID            string
	Language          string
	ReminderFrequency string
	AgentNotes        string
	LastInteraction   *time.Time
}

// GetUserContext 加载用户上下文。如果未找到则返回默认值。
func (s *Store) GetUserContext(ctx context.Context, userID string) (*UserContext, error) {
	uc := &UserContext{
		UserID:            userID,
		Language:          "zh",
		ReminderFrequency: "weekly",
	}

	var lastInteraction sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT language, reminder_frequency, agent_notes, last_interaction
		 FROM user_context WHERE user_id = ?`, userID,
	).Scan(&uc.Language, &uc.ReminderFrequency, &uc.AgentNotes, &lastInteraction)

	if err == sql.ErrNoRows {
		return uc, nil // 返回默认值
	}
	if err != nil {
		return uc, nil // 出错时也返回默认值
	}

	if lastInteraction.Valid {
		uc.LastInteraction = &lastInteraction.Time
	}
	return uc, nil
}

// SaveUserContext 持久化用户上下文。
func (s *Store) SaveUserContext(ctx context.Context, uc *UserContext) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_context (user_id, language, reminder_frequency, agent_notes, last_interaction, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET
		   language = excluded.language,
		   reminder_frequency = excluded.reminder_frequency,
		   agent_notes = excluded.agent_notes,
		   last_interaction = CURRENT_TIMESTAMP,
		   updated_at = CURRENT_TIMESTAMP`,
		uc.UserID, uc.Language, uc.ReminderFrequency, uc.AgentNotes)
	if err != nil {
		return fmt.Errorf("save user context: %w", err)
	}
	return nil
}

// TouchUserInteraction 更新最后交互时间戳。
func (s *Store) TouchUserInteraction(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_context (user_id, last_interaction, updated_at) VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET last_interaction = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP`,
		userID)
	return err
}

// SchedulerState 保存每个用户的调度状态。
type SchedulerState struct {
	UserID          string
	LastCheck       *time.Time
	RiskLevel       string
	DaysSinceUpdate int
	LastReminder    *time.Time
}

// GetSchedulerState 加载用户的调度状态。
func (s *Store) GetSchedulerState(ctx context.Context, userID string) (*SchedulerState, error) {
	ss := &SchedulerState{UserID: userID, RiskLevel: "normal"}

	var lastCheck, lastReminder sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT last_check, risk_level, days_since_update, last_reminder
		 FROM scheduler_state WHERE user_id = ?`, userID,
	).Scan(&lastCheck, &ss.RiskLevel, &ss.DaysSinceUpdate, &lastReminder)

	if err != nil {
		return ss, nil // 返回默认值
	}
	if lastCheck.Valid {
		ss.LastCheck = &lastCheck.Time
	}
	if lastReminder.Valid {
		ss.LastReminder = &lastReminder.Time
	}
	return ss, nil
}

// SaveSchedulerState 持久化调度状态。
func (s *Store) SaveSchedulerState(ctx context.Context, ss *SchedulerState) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scheduler_state (user_id, last_check, risk_level, days_since_update, last_reminder, updated_at)
		 VALUES (?, CURRENT_TIMESTAMP, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET
		   last_check = CURRENT_TIMESTAMP,
		   risk_level = excluded.risk_level,
		   days_since_update = excluded.days_since_update,
		   last_reminder = excluded.last_reminder,
		   updated_at = CURRENT_TIMESTAMP`,
		ss.UserID, ss.RiskLevel, ss.DaysSinceUpdate, ss.LastReminder)
	if err != nil {
		return fmt.Errorf("save scheduler state: %w", err)
	}
	return nil
}

// UpdateReminderTime 标记提醒已发送。
func (s *Store) UpdateReminderTime(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scheduler_state (user_id, last_reminder, updated_at) VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET last_reminder = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP`,
		userID)
	return err
}
