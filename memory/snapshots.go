package memory

import (
	"context"
	"fmt"
	"time"
)

// OKRSnapshot 表示用户 OKR 数据的时间点快照。
type OKRSnapshot struct {
	ID        int64
	UserID    string
	Month     string
	OKRData   string // 格式化的 OKR 文本
	CreatedAt time.Time
}

// SaveOKRSnapshot 存储 OKR 快照，用于趋势分析。
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
func (s *Store) GetOKRSnapshots(ctx context.Context, userID string, limit int) ([]OKRSnapshot, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, month, okr_data, created_at
		 FROM okr_snapshots WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

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
