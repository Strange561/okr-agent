package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"okr-agent/llm"
)

// conversationTTL 定义对话记录的过期时间，设为 24 小时。
//
// 选择 24 小时作为 TTL 的原因：
//   - OKR 相关的对话通常在同一天内完成
//   - 过长的对话历史会增加 LLM 的输入 token 消耗（增加成本）
//   - 隔天的上下文关联性较低，不如重新开始对话
//   - 避免积累过多的过期数据占用存储空间
const conversationTTL = 24 * time.Hour

// SaveConversation 持久化指定用户的对话记录。
//
// 使用 SQLite 的 UPSERT（INSERT ... ON CONFLICT DO UPDATE）语法，
// 确保每个用户始终只保留一条记录。当用户有新消息时，整个对话历史
// 会被序列化为 JSON 并覆盖写入。
//
// 参数：
//   - userID: 用户的 open_id，作为主键唯一标识
//   - messages: 完整的对话消息列表，包含所有角色的消息
//
// 在 Agent 架构中，对话持久化使得用户可以跨 HTTP 请求维持上下文。
// 例如用户先问"帮我看看 OKR"，然后追问"第二个 KR 怎么改进"，
// Agent 能理解"第二个 KR"指的是之前查询到的 OKR 中的某个 KR。
func (s *Store) SaveConversation(ctx context.Context, userID string, messages []llm.Message) error {
	// 将消息列表序列化为 JSON 字符串存储
	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	// UPSERT：如果用户已有对话记录则更新，否则插入新记录
	// 同时更新 updated_at 时间戳，用于 TTL 过期判断
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
//
// 工作流程：
//  1. 根据 user_id 查询对话记录
//  2. 如果未找到（新用户），返回 nil 表示需要开始新对话
//  3. 检查 updated_at 是否超过 TTL（24 小时）
//  4. 如果已过期，自动删除过期记录并返回 nil
//  5. 如果未过期，反序列化 JSON 并返回消息列表
//
// 返回 nil 而不是 error 的设计，使得调用方可以简单地用 nil 检查来判断
// 是否需要开始新对话，无需额外的错误处理逻辑。
func (s *Store) GetConversation(ctx context.Context, userID string) ([]llm.Message, error) {
	var messagesJSON string
	var updatedAt time.Time

	// 查询指定用户的对话记录
	err := s.db.QueryRowContext(ctx,
		`SELECT messages, updated_at FROM conversations WHERE user_id = ?`, userID,
	).Scan(&messagesJSON, &updatedAt)

	if err != nil {
		return nil, nil // 未找到 → 新对话，返回 nil 让调用方初始化空对话
	}

	// 检查对话是否已过期（超过 24 小时）
	if time.Since(updatedAt) > conversationTTL {
		// 已过期 — 删除旧记录并返回 nil，相当于重置对话
		s.db.ExecContext(ctx, `DELETE FROM conversations WHERE user_id = ?`, userID)
		return nil, nil
	}

	// 反序列化 JSON 为消息列表
	var messages []llm.Message
	if err := json.Unmarshal([]byte(messagesJSON), &messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return messages, nil
}

// DeleteConversation 删除一条对话记录。
// 可用于手动清除用户的对话上下文（如用户请求"重新开始"）。
func (s *Store) DeleteConversation(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE user_id = ?`, userID)
	return err
}
