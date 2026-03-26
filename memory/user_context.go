package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UserContext holds user preferences and agent-maintained notes.
type UserContext struct {
	UserID            string
	Language          string
	ReminderFrequency string
	AgentNotes        string
	LastInteraction   *time.Time
}

// GetUserContext loads user context. Returns defaults if not found.
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
		return uc, nil // return defaults
	}
	if err != nil {
		return uc, nil // return defaults on error too
	}

	if lastInteraction.Valid {
		uc.LastInteraction = &lastInteraction.Time
	}
	return uc, nil
}

// SaveUserContext persists user context.
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

// TouchUserInteraction updates the last_interaction timestamp.
func (s *Store) TouchUserInteraction(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_context (user_id, last_interaction, updated_at) VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET last_interaction = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP`,
		userID)
	return err
}

// SchedulerState holds per-user scheduling state.
type SchedulerState struct {
	UserID          string
	LastCheck       *time.Time
	RiskLevel       string
	DaysSinceUpdate int
	LastReminder    *time.Time
}

// GetSchedulerState loads the scheduler state for a user.
func (s *Store) GetSchedulerState(ctx context.Context, userID string) (*SchedulerState, error) {
	ss := &SchedulerState{UserID: userID, RiskLevel: "normal"}

	var lastCheck, lastReminder sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT last_check, risk_level, days_since_update, last_reminder
		 FROM scheduler_state WHERE user_id = ?`, userID,
	).Scan(&lastCheck, &ss.RiskLevel, &ss.DaysSinceUpdate, &lastReminder)

	if err != nil {
		return ss, nil // defaults
	}
	if lastCheck.Valid {
		ss.LastCheck = &lastCheck.Time
	}
	if lastReminder.Valid {
		ss.LastReminder = &lastReminder.Time
	}
	return ss, nil
}

// SaveSchedulerState persists scheduler state.
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

// UpdateReminderTime marks that a reminder was sent.
func (s *Store) UpdateReminderTime(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scheduler_state (user_id, last_reminder, updated_at) VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET last_reminder = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP`,
		userID)
	return err
}
