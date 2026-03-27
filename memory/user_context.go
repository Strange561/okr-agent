package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// UserContext 保存用户偏好和 Agent 维护的备注信息。
//
// 在 Agent 架构中，UserContext 是"长期记忆"的一部分。
// 与对话记录（短期记忆，24 小时过期）不同，UserContext 是持久的，
// 帮助 Agent 在不同会话之间保持对用户的了解。
//
// 典型使用场景：
//   - 记住用户偏好的语言（中文/英文），在系统提示词中使用
//   - 记录 Agent 对该用户的观察（如"该用户经常延迟更新 KR3"），用于个性化建议
//   - 追踪最后交互时间，用于判断用户活跃度
type UserContext struct {
	UserID            string     // 用户的 open_id，作为唯一标识
	Language          string     // 用户偏好语言，"zh"（中文）或 "en"（英文）
	ReminderFrequency string     // 提醒频率设置，如 "weekly"、"daily"
	AgentNotes        string     // Agent 维护的用户备注，可跨会话积累
	LastInteraction   *time.Time // 用户最后一次与 Agent 交互的时间（指针类型，因为可能为空）
}

// GetUserContext 加载用户上下文。如果未找到则返回默认值。
//
// 设计决策：即使数据库中没有该用户的记录，也返回带有默认值的 UserContext，
// 而不是返回错误。这简化了调用方的逻辑——调用方不需要处理"用户不存在"的情况。
// 默认值为：语言=zh，提醒频率=weekly。
func (s *Store) GetUserContext(ctx context.Context, userID string) (*UserContext, error) {
	// 先构建带默认值的 UserContext，即使查询失败也能返回合理的结果
	uc := &UserContext{
		UserID:            userID,
		Language:          "zh",
		ReminderFrequency: "weekly",
	}

	// 使用 sql.NullTime 处理可能为 NULL 的时间字段
	var lastInteraction sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT language, reminder_frequency, agent_notes, last_interaction
		 FROM user_context WHERE user_id = ?`, userID,
	).Scan(&uc.Language, &uc.ReminderFrequency, &uc.AgentNotes, &lastInteraction)

	if err == sql.ErrNoRows {
		return uc, nil // 用户不存在于数据库中，返回默认值
	}
	if err != nil {
		return uc, nil // 查询出错时也返回默认值，保证系统可用性（降级处理）
	}

	// 如果 last_interaction 有效（非 NULL），则设置到 UserContext 中
	if lastInteraction.Valid {
		uc.LastInteraction = &lastInteraction.Time
	}
	return uc, nil
}

// SaveUserContext 持久化用户上下文。
//
// 使用 UPSERT 策略：如果用户已有记录则更新，否则插入新记录。
// 每次保存都会更新 last_interaction 和 updated_at 为当前时间。
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
//
// 这是一个轻量级的操作，只更新 last_interaction 时间，
// 不修改其他用户设置。每次用户与 Agent 交互时都会调用此方法，
// 用于追踪用户活跃度。
//
// 使用 UPSERT 确保即使用户没有 user_context 记录也能正常工作——
// 首次交互时会自动创建一条默认记录。
func (s *Store) TouchUserInteraction(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_context (user_id, last_interaction, updated_at) VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET last_interaction = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP`,
		userID)
	return err
}

// SchedulerState 保存每个用户的调度状态。
//
// 调度器使用此结构体来跟踪 OKR 更新的风险状况，并控制提醒频率。
// 风险等级和上次提醒时间的组合，确保不会过于频繁地打扰用户，
// 同时又能在关键时刻（如长时间未更新）及时发出提醒。
type SchedulerState struct {
	UserID          string     // 用户的 open_id
	LastCheck       *time.Time // 上次调度器检查该用户的时间
	RiskLevel       string     // 风险等级：normal / high / critical
	DaysSinceUpdate int        // 距离上次 OKR 更新的天数
	LastReminder    *time.Time // 上次发送提醒的时间（用于控制提醒频率）
}

// GetSchedulerState 加载用户的调度状态。
//
// 与 GetUserContext 类似，如果未找到记录则返回默认值（risk_level="normal"）。
// 确保调度器即使在首次运行时也能正常工作。
func (s *Store) GetSchedulerState(ctx context.Context, userID string) (*SchedulerState, error) {
	// 初始化默认状态：风险等级为 normal
	ss := &SchedulerState{UserID: userID, RiskLevel: "normal"}

	// 使用 NullTime 处理可能为 NULL 的时间字段
	var lastCheck, lastReminder sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT last_check, risk_level, days_since_update, last_reminder
		 FROM scheduler_state WHERE user_id = ?`, userID,
	).Scan(&lastCheck, &ss.RiskLevel, &ss.DaysSinceUpdate, &lastReminder)

	if err != nil {
		return ss, nil // 未找到或查询出错，返回默认值
	}
	// 设置有效的时间字段
	if lastCheck.Valid {
		ss.LastCheck = &lastCheck.Time
	}
	if lastReminder.Valid {
		ss.LastReminder = &lastReminder.Time
	}
	return ss, nil
}

// SaveSchedulerState 持久化调度状态。
//
// 使用 UPSERT 策略，在每次风险扫描后更新状态。
// last_check 自动设为当前时间（CURRENT_TIMESTAMP），
// 其他字段从传入的 SchedulerState 结构体中获取。
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
//
// 当调度器成功发送提醒后调用此方法，更新 last_reminder 时间。
// 后续的风险扫描会检查 last_reminder 来判断是否需要再次提醒，
// 从而避免频繁打扰用户。
func (s *Store) UpdateReminderTime(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scheduler_state (user_id, last_reminder, updated_at) VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET last_reminder = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP`,
		userID)
	return err
}
