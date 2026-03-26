package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store 管理基于 SQLite 的持久化存储。
type Store struct {
	db *sql.DB
}

// NewStore 打开（或创建）SQLite 数据库并执行迁移。
func NewStore(dbPath string) (*Store, error) {
	// 确保父目录存在
	if dir := filepath.Dir(dbPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// WAL 模式 + 单连接，确保并发安全
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			user_id TEXT PRIMARY KEY,
			messages TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS okr_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			month TEXT NOT NULL,
			okr_data TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_okr_snapshots_user ON okr_snapshots(user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS evaluation_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			evaluation TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_evaluation_history_user ON evaluation_history(user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS user_context (
			user_id TEXT PRIMARY KEY,
			language TEXT DEFAULT 'zh',
			reminder_frequency TEXT DEFAULT 'weekly',
			agent_notes TEXT DEFAULT '',
			last_interaction DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS scheduler_state (
			user_id TEXT PRIMARY KEY,
			last_check DATETIME,
			risk_level TEXT DEFAULT 'normal',
			days_since_update INTEGER DEFAULT 0,
			last_reminder DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("exec migration: %w\nSQL: %s", err, m)
		}
	}
	return nil
}
