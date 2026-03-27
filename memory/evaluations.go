package memory

import (
	"context"
	"fmt"
	"time"
)

// Evaluation 表示已存储的评估记录。
//
// 每次 Agent 对用户的 OKR 进行评估后，评估结果可以被保存到数据库中。
// 保存评估记录的目的：
//   - 允许用户回顾历史评估，了解自己 OKR 的改进趋势
//   - Agent 可以参考历史评估避免重复给出相同建议
//   - 管理者可以查看团队成员的评估历史
type Evaluation struct {
	ID         int64     // 数据库自增主键
	UserID     string    // 被评估用户的 open_id
	Evaluation string    // 评估内容文本（由 LLM 生成的自然语言评估）
	CreatedAt  time.Time // 评估创建时间
}

// SaveEvaluation 存储评估结果。
//
// 每次调用都会插入一条新记录，保留完整的评估历史。
// 这与对话记录（每用户只保留最新一条）不同，
// 因为评估历史本身就是有价值的长期数据。
//
// 参数：
//   - userID: 被评估用户的 open_id
//   - evaluation: 评估内容文本
func (s *Store) SaveEvaluation(ctx context.Context, userID, evaluation string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO evaluation_history (user_id, evaluation) VALUES (?, ?)`,
		userID, evaluation)
	if err != nil {
		return fmt.Errorf("save evaluation: %w", err)
	}
	return nil
}

// GetEvaluations 返回用户最近的评估记录。
//
// limit 参数控制返回的最大记录数。如果传入 0 或负数，默认返回 5 条。
// 选择默认 5 条是因为评估内容通常较长，过多会导致 LLM 上下文过大。
// 返回结果按 created_at 降序排列，即最新的评估排在最前面。
func (s *Store) GetEvaluations(ctx context.Context, userID string, limit int) ([]Evaluation, error) {
	// 如果未指定 limit 或为非正数，使用默认值 5
	if limit <= 0 {
		limit = 5
	}

	// 按时间倒序查询最近的评估记录
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, evaluation, created_at
		 FROM evaluation_history WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query evaluations: %w", err)
	}
	defer rows.Close()

	// 逐行扫描结果并构建评估列表
	var evals []Evaluation
	for rows.Next() {
		var e Evaluation
		if err := rows.Scan(&e.ID, &e.UserID, &e.Evaluation, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan evaluation: %w", err)
		}
		evals = append(evals, e)
	}
	return evals, rows.Err()
}
