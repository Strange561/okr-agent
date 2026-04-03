# OKR Agent 十项优化修复方案

## Context

基于对 okr-agent 全部代码的审查，发现 1 个 bug 和 9 个优化点。其中对话历史 bug 直接影响多轮对话体验，token 缓存和 LLM 超时影响可靠性，其余为架构和性能改进。

## 实施顺序（按依赖关系）

**Phase 1 — 独立基础设施修复（无依赖）**
- #3 LLM 超时 → `llm/client.go`
- #2 Token 缓存 → `feishu/client.go` + `feishu/okr.go`

**Phase 2 — Agent 重构（有依赖链：#6→#1→#5→#9→#10）**
- #6 接口化 → `agent/agent.go` + `memory/interfaces.go`
- #1 修复对话保存 bug → `agent/agent.go`（reactLoop 返回更新后的 messages）
- #5 智能截断 → `agent/agent.go`（truncateHistory 不切断 tool 序列）
- #9 清除 ReasoningContent → `agent/agent.go`（保存前 strip）
- #10 用户级并发控制 → `agent/agent.go`（sync.Map + per-user mutex）

**Phase 3 — 调度器和存储改进**
- #8 快照清理 → `memory/snapshots.go` + `config/config.go` + `scheduler/scheduler.go`
- #4 避免重复获取 OKR → `scheduler/scheduler.go`（预获取嵌入 prompt）
- #7 调度器并发处理 → `scheduler/scheduler.go`（semaphore, max 3 并发）

## 各项具体改动

### #1 BUG: 对话历史未保存 Agent 回复
- **文件**: `agent/agent.go`
- **问题**: `reactLoop` 内 `append` 扩容后调用方 `conversation` 看不到新消息
- **改动**: `reactLoop` 签名改为返回 `(*RunResult, []llm.Message, error)`，`Run()` 用返回的 messages 保存

### #2 Tenant Access Token 未缓存
- **文件**: `feishu/client.go` + `feishu/okr.go`
- **改动**: Client 增加 `tokenCache string` / `tokenExpiry time.Time` / `tokenMu sync.Mutex`；`getTenantAccessToken` 先查缓存，未过期直接返回，过期才请求（提前 60s 失效）

### #3 LLM HTTP 客户端无超时
- **文件**: `llm/client.go`
- **改动**: `&http.Client{}` → `&http.Client{Timeout: 120 * time.Second}`

### #4 DailyRiskScan 重复获取 OKR
- **文件**: `scheduler/scheduler.go`
- **改动**: 将已获取的 OKR 数据格式化后嵌入 prompt，注明"已获取，无需再次查询"；同样优化 `RunCheck()`

### #5 历史截断切断 tool_calls 序列
- **文件**: `agent/agent.go`
- **改动**: `truncateHistory` 截断后如果落在 tool/assistant(tool_calls) 消息上，向前推进到下一个 user 或无 tool_calls 的 assistant 消息

### #6 Agent 使用具体类型而非接口
- **文件**: `agent/agent.go` + `memory/interfaces.go`
- **改动**: Agent struct 字段改为 `llm.ChatClient` 和 `memory.AgentStore`（新组合接口 = ConversationStore + UserContextStore）

### #7 调度器串行处理用户
- **文件**: `scheduler/scheduler.go`
- **改动**: `RunCheck` 和 `DailyRiskScan` 改用 `sync.WaitGroup` + `chan struct{}` semaphore（max 3 并发）

### #8 okr_snapshots 表无限增长
- **文件**: `memory/snapshots.go` + `memory/interfaces.go` + `config/config.go` + `scheduler/scheduler.go`
- **改动**: 新增 `CleanupOldSnapshots` 方法；config 加 `SnapshotRetentionDays`（默认 90）；scheduler 加每日 3:00 AM 清理 cron

### #9 ReasoningContent 污染对话历史
- **文件**: `agent/agent.go`
- **改动**: 新增 `stripReasoningContent` 函数，`SaveConversation` 前调用

### #10 无用户级并发控制
- **文件**: `agent/agent.go`
- **改动**: Agent struct 加 `userMu sync.Map`；`Run()` 开头 `getUserLock(userID).Lock()`；`RunOneShot` 不需要

## 文件改动汇总

| 文件 | 涉及 Issue |
|------|-----------|
| `agent/agent.go` | #1, #5, #6, #9, #10 |
| `llm/client.go` | #3 |
| `feishu/client.go` | #2 |
| `feishu/okr.go` | #2 |
| `memory/interfaces.go` | #6, #8 |
| `memory/snapshots.go` | #8 |
| `scheduler/scheduler.go` | #4, #7, #8 |
| `config/config.go` | #8 |
| `main.go` | 无需改动（具体类型自动满足接口） |
