package memory

import (
	"context"
	"fmt"
	"time"
)

// OKRSnapshot 表示用户 OKR 数据的时间点快照。
//
// 快照是 Agent 实现"趋势分析"和"进度对比"功能的基础。
// 每次通过工具获取用户 OKR 数据时，系统会自动保存一份快照。
// 通过对比不同时间点的快照，Agent 可以分析出：
//   - OKR 进度是否有变化
//   - 进度更新的频率和节奏
//   - 某段时间内的进展速度是否正常
type OKRSnapshot struct {
	ID        int64     // 数据库自增主键
	UserID    string    // 用户的 open_id
	Month     string    // OKR 所属月份，格式 "YYYY-MM"
	OKRData   string    // 格式化的 OKR 文本数据（由 FormatOKRForEvaluation 生成）
	CreatedAt time.Time // 快照创建时间
}

// SaveOKRSnapshot 存储 OKR 快照，用于趋势分析。
//
// 注意：每次调用都会插入一条新记录（不是更新），这样可以保留完整的历史。
// 一个用户在同一个月份可能有多条快照记录，代表不同时间点获取的 OKR 状态。
//
// 参数：
//   - userID: 用户的 open_id
//   - month: OKR 所属月份，格式 "YYYY-MM"
//   - okrData: 格式化的 OKR 文本（人类和 LLM 可读的格式）
func (s *Store) SaveOKRSnapshot(ctx context.Context, userID, month, okrData string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO okr_snapshots (user_id, month, okr_data) VALUES (?, ?, ?)`,
		userID, month, okrData)
	if err != nil {
		return fmt.Errorf("save OKR snapshot: %w", err)
	}
	return nil
}

// GetOKRSnapshots 返回用户最近的快照，按时间倒序排列。
//
// limit 参数控制返回的最大记录数。如果传入 0 或负数，默认返回 10 条。
// 返回结果按 created_at 降序排列，即最新的快照排在最前面。
//
// 使用场景：
//   - Agent 的 get_okr_history 工具调用此方法获取历史数据
//   - 趋势分析时对比多个时间点的 OKR 状态
func (s *Store) GetOKRSnapshots(ctx context.Context, userID string, limit int) ([]OKRSnapshot, error) {
	// 如果未指定 limit 或为非正数，使用默认值 10
	if limit <= 0 {
		limit = 10
	}

	// 按时间倒序查询，最新的记录排在最前面
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, month, okr_data, created_at
		 FROM okr_snapshots WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	// 逐行扫描结果并构建快照列表
	var snapshots []OKRSnapshot
	for rows.Next() {
		var snap OKRSnapshot
		if err := rows.Scan(&snap.ID, &snap.UserID, &snap.Month, &snap.OKRData, &snap.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, rows.Err()
}

// CleanupOldSnapshots 删除超过指定天数的旧快照。
func (s *Store) CleanupOldSnapshots(ctx context.Context, retentionDays int) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM okr_snapshots WHERE created_at < datetime('now', ? || ' days')`,
		fmt.Sprintf("%d", -retentionDays))
	if err != nil {
		return 0, fmt.Errorf("cleanup old snapshots: %w", err)
	}
	return result.RowsAffected()
}
