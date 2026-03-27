// Package memory 提供基于 SQLite 的持久化存储层。
//
// 在 Agent 架构中，memory（记忆）是让 Agent 具有"状态"的关键组件。
// 没有记忆的 Agent 每次交互都是独立的，无法进行上下文延续、趋势分析等高级功能。
//
// 本包管理以下数据：
//   - conversations：用户的对话历史，支持多轮对话
//   - okr_snapshots：OKR 数据的时间点快照，用于趋势分析
//   - evaluation_history：Agent 生成的评估记录
//   - user_context：用户偏好和 Agent 备注
//   - scheduler_state：调度器状态，控制提醒频率
//
// 使用 SQLite 作为存储引擎，兼顾轻量级部署和 ACID 事务保证。
// 采用纯 Go 实现的 modernc.org/sqlite 驱动，无需 CGO。
package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // 导入纯 Go 的 SQLite 驱动，通过 database/sql 接口注册
)

// Store 管理基于 SQLite 的持久化存储。
//
// Store 是所有数据访问操作的入口点。它封装了 database/sql.DB 连接，
// 并通过各种方法提供类型安全的数据访问接口。
// 在整个应用中通常只创建一个 Store 实例，由 main 函数初始化后传递给各组件。
type Store struct {
	db *sql.DB // SQLite 数据库连接，所有 CRUD 操作都通过此连接执行
}

// NewStore 打开（或创建）SQLite 数据库并执行迁移。
//
// 工作流程：
//  1. 确保数据库文件的父目录存在（自动创建）
//  2. 使用纯 Go 的 SQLite 驱动打开数据库连接
//  3. 配置 WAL 模式和单连接，确保并发安全
//  4. 执行数据库迁移，创建所需的表和索引
//
// dbPath 参数指定 SQLite 数据库文件的路径。
// 如果文件不存在会自动创建；如果已存在则直接打开。
func NewStore(dbPath string) (*Store, error) {
	// 确保父目录存在，例如 "./data/" 目录
	// 这样用户不需要手动创建目录就能使用
	if dir := filepath.Dir(dbPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}

	// 使用 "sqlite" 驱动名打开数据库连接
	// modernc.org/sqlite 是纯 Go 实现，无需 CGO 编译
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// WAL（Write-Ahead Logging）模式提供更好的并发读写性能。
	// 设置 MaxOpenConns=1 确保同一时间只有一个写操作，
	// 避免 SQLite 的 "database is locked" 错误。
	// 这对于单实例部署的 Agent 服务来说是最稳妥的策略。
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	s := &Store{db: db}
	// 执行数据库迁移，创建或更新表结构
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// Close 关闭数据库连接。
// 应在应用退出时调用（通常通过 defer store.Close()）。
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate 执行数据库迁移，创建所有必要的表和索引。
//
// 采用"幂等迁移"策略：每条 SQL 都使用 IF NOT EXISTS，
// 因此可以安全地多次执行而不会出错。这种方式适合简单项目，
// 对于更复杂的迁移需求可以考虑使用专门的迁移工具。
//
// 创建的表说明：
//   - conversations：存储用户对话历史，每个用户一条记录（UPSERT 方式更新）
//   - okr_snapshots：存储 OKR 数据快照，每次查询 OKR 时自动保存，用于趋势分析
//   - evaluation_history：存储 Agent 生成的评估结果，便于回顾历史评估
//   - user_context：存储用户偏好（语言、提醒频率）和 Agent 的备注信息
//   - scheduler_state：存储调度器的运行状态，控制提醒频率和风险等级
func (s *Store) migrate() error {
	migrations := []string{
		// conversations 表：用户对话历史
		// user_id 为主键，表示每个用户只保留一条最新的对话记录
		// messages 以 JSON 格式存储完整的对话消息列表
		`CREATE TABLE IF NOT EXISTS conversations (
			user_id TEXT PRIMARY KEY,
			messages TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// okr_snapshots 表：OKR 数据的时间点快照
		// 每次获取 OKR 数据时自动保存一份快照
		// 通过对比不同时间的快照，可以分析 OKR 进度变化趋势
		`CREATE TABLE IF NOT EXISTS okr_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			month TEXT NOT NULL,
			okr_data TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 为 okr_snapshots 创建索引，加速按用户查询和时间排序
		`CREATE INDEX IF NOT EXISTS idx_okr_snapshots_user ON okr_snapshots(user_id, created_at DESC)`,
		// evaluation_history 表：评估历史记录
		// 保存 Agent 对用户 OKR 的评估结果，供后续参考
		`CREATE TABLE IF NOT EXISTS evaluation_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			evaluation TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 为 evaluation_history 创建索引，加速按用户查询和时间排序
		`CREATE INDEX IF NOT EXISTS idx_evaluation_history_user ON evaluation_history(user_id, created_at DESC)`,
		// user_context 表：用户偏好和 Agent 备注
		// language: 用户偏好语言，默认中文
		// reminder_frequency: 提醒频率设置
		// agent_notes: Agent 记录的关于该用户的备注信息（持久化记忆）
		`CREATE TABLE IF NOT EXISTS user_context (
			user_id TEXT PRIMARY KEY,
			language TEXT DEFAULT 'zh',
			reminder_frequency TEXT DEFAULT 'weekly',
			agent_notes TEXT DEFAULT '',
			last_interaction DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// scheduler_state 表：调度器状态管理
		// 追踪每个用户的 OKR 更新风险等级和上次提醒时间，
		// 防止过于频繁地发送提醒消息
		`CREATE TABLE IF NOT EXISTS scheduler_state (
			user_id TEXT PRIMARY KEY,
			last_check DATETIME,
			risk_level TEXT DEFAULT 'normal',
			days_since_update INTEGER DEFAULT 0,
			last_reminder DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	// 依次执行每条迁移 SQL
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("exec migration: %w\nSQL: %s", err, m)
		}
	}
	return nil
}
