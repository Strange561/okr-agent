# OKR Agent

智能 OKR Agent。通过飞书 Bot 与团队成员自然语言对话，自主调用工具查询 OKR 数据、进行多维度评估、发送个性化提醒，处理文档评论，并根据风险信号动态调度干预。

## 核心能力

- **自然语言交互** — 不再依赖固定命令，直接用自然语言对话（"帮我看看张三的 OKR"、"对比一下上个月和这个月的进展"）
- **Agent 自主推理** — ReAct 循环 + Tool Calling，LLM 自主决定调用哪些工具、如何组合、何时停止
- **对话记忆** — 按用户隔离的对话历史，24 小时内多轮对话保持上下文
- **智能调度** — 每日风险扫描 + 风险升级（7→14→21 天），Agent 生成个性化提醒
- **OKR 趋势分析** — 每次查询自动快照，支持历史对比和进展趋势
- **文档评论处理** — 读取飞书文档评论，理解上下文，修改文档内容，回复评论说明修改

## 架构

```
用户消息 → 飞书 Bot → Agent ReAct 循环 → LLM（Kimi K2.5，带工具定义）
                                ↕
                        工具执行（OKR 查询、消息发送、团队查询、文档操作）
                                ↕
                        SQLite 记忆（对话、快照、评估、用户偏好）
```

### Agent 循环

```
用户消息
    → 加载对话历史（SQLite, 24h TTL）
    → 构建系统提示词（注入用户偏好和上下文）
    → 调用 LLM（带 13 个工具定义）
    → 检查 finish_reason:
        tool_calls → 执行工具 → 追加结果 → 重新调用（最多 10 轮）
        stop → 提取文本 → 保存对话 → 返回响应
```

## 项目结构

```
okr-agent/
├── main.go                     # 入口：初始化 store → LLM → tools → agent → scheduler → bot
├── config/config.go            # 配置加载（.env / 环境变量）
├── agent/
│   ├── agent.go                # ReAct 循环核心（Run + RunOneShot）
│   ├── types.go                # RunResult 类型
│   └── system_prompt.go        # OKR 助手系统提示词（含文档评论处理工作流）
├── llm/
│   ├── client.go               # OpenAI 兼容 Chat Completion HTTP 客户端（Kimi/Moonshot）
│   ├── types.go                # Request/Response/Tool/Message 类型定义
│   └── interfaces.go           # ChatClient 接口定义
├── tools/
│   ├── tool.go                 # Tool 接口定义
│   ├── registry.go             # 工具注册 + OpenAI ToolParam 转换 + 分发执行
│   ├── okr_query.go            # get_user_okrs, get_okr_history, compare_okr_periods
│   ├── messaging.go            # send_message, send_reminder, send_team_notification
│   ├── okr_ops.go              # update_okr_progress（桩，引导手动操作）
│   ├── user_query.go           # list_team_members
│   └── document.go             # list_doc_comments, get_doc_content, list_doc_blocks, update_doc_block, reply_doc_comment
├── memory/
│   ├── store.go                # SQLite 连接 + schema 迁移（5 张表）
│   ├── conversations.go        # 对话持久化（24h 过期）
│   ├── snapshots.go            # OKR 快照存储
│   ├── evaluations.go          # 评估历史
│   ├── user_context.go         # 用户偏好 + 调度状态
│   └── interfaces.go           # 存储层接口定义
├── feishu/
│   ├── client.go               # 飞书客户端初始化
│   ├── user.go                 # 用户管理（静态 ID + 部门 API）
│   ├── okr.go                  # OKR API 调用与数据格式化
│   ├── message.go              # 发送消息（文本 / 富文本）
│   ├── document.go             # 文档操作（评论、内容、块编辑、Wiki 解析）
│   ├── bot.go                  # Bot WebSocket 事件处理（含去重清理）
│   └── interfaces.go           # 飞书服务接口定义（OKR/消息/用户/文档）
├── scheduler/scheduler.go      # 定时任务：主检查 + 每日风险扫描 + 周五提醒
├── Dockerfile                  # 多阶段构建，VOLUME /app/data
└── .env.example                # 配置模板
```

## 工具列表

Agent 可自主调用以下 13 个工具：

### OKR 查询

| 工具 | 说明 | 底层调用 |
|------|------|---------|
| `get_user_okrs` | 获取用户 OKR 数据（自动保存快照） | `feishu.GetUserOKRs` |
| `get_okr_history` | 查询历史快照，用于趋势分析 | `memory.GetOKRSnapshots` |
| `compare_okr_periods` | 对比两个月份的 OKR 数据 | `feishu.GetUserOKRs` ×2 |

### 消息通知

| 工具 | 说明 | 底层调用 |
|------|------|---------|
| `send_message` | 向用户发送文本消息 | `feishu.SendTextMessage` |
| `send_reminder` | 向用户发送富文本提醒 | `feishu.SendRichMessage` |
| `send_team_notification` | 向所有团队成员广播 | `feishu.CollectUsers` + 循环发送 |

### 团队 & OKR 操作

| 工具 | 说明 | 底层调用 |
|------|------|---------|
| `list_team_members` | 列出所有监控的团队成员 | `feishu.CollectUsers` |
| `update_okr_progress` | 更新 OKR 进度（桩） | 引导用户在飞书手动操作 |

### 文档操作

| 工具 | 说明 | 底层调用 |
|------|------|---------|
| `list_doc_comments` | 获取文档评论（支持 docx/wiki/doc/sheet） | `feishu.ListDocComments` |
| `get_doc_content` | 获取文档全文内容 | `feishu.GetDocContent` |
| `list_doc_blocks` | 列出文档块 ID 和文本内容 | `feishu.ListDocBlocks` |
| `update_doc_block` | 修改指定块的文本内容 | `feishu.UpdateDocBlock` |
| `reply_doc_comment` | 回复文档评论（支持 docx/wiki/doc/sheet） | `feishu.ReplyToComment` |

## 定时任务

| 任务 | 时间 | 行为 |
|------|------|------|
| 主检查 | 可配置（默认周一 9:00） | Agent 驱动的批量 OKR 评估，自动发送评价给每位成员 |
| 每日风险扫描 | 每天 10:00 | 检查所有用户 OKR 更新时间，触发风险升级 |
| 周五提醒 | 每周五 10:00 | 发送固定格式的 OKR 更新提醒 |

### 风险升级逻辑

| 未更新天数 | 风险等级 | 动作 |
|-----------|---------|------|
| 7+ 天 | `normal` | 由周五提醒覆盖 |
| 14+ 天 | `high` | 3 天内未提醒过 → Agent 生成个性化提醒 |
| 21+ 天 | `critical` | 每天检查 → Agent 生成升级通知 |

## 记忆系统

使用 SQLite（`modernc.org/sqlite` 纯 Go，零 CGO）持久化以下数据：

| 表 | 用途 |
|----|------|
| `conversations` | 对话历史（JSON），24h 不活跃自动过期 |
| `okr_snapshots` | OKR 快照，每次 `get_user_okrs` 自动保存 |
| `evaluation_history` | 评估记录存档 |
| `user_context` | 用户偏好（语言、提醒频率、Agent 备注） |
| `scheduler_state` | 调度状态（上次检查、风险等级、上次提醒时间） |

## 快速开始

### 前置条件

- Go 1.25+
- [飞书开放平台](https://open.feishu.cn/)应用
- 兼容 OpenAI 格式的 LLM API（默认使用 [Kimi K2.5](https://platform.moonshot.cn/)，也支持 OpenAI、Azure OpenAI 等）

### 飞书应用权限

| 权限 | 用途 |
|------|------|
| `okr:okr:readonly` | 读取用户 OKR 数据 |
| `im:message:send_as_bot` | 以机器人身份发送消息 |
| `im:message.receive_v1` | 接收用户发给机器人的消息 |
| `contact:user.id:readonly` | 读取用户 ID |
| `contact:department.member:readonly` | 通过部门 ID 获取成员列表 |
| `drive:drive.comment:write` | 读写文档评论 |
| `docx:document:write` | 读写飞书文档内容 |
| `wiki:wiki:readonly` | 读取知识库 Wiki 页面（解析 wiki token） |

### 安装与运行

```bash
git clone https://github.com/Strange561/okr-agent.git
cd okr-agent

cp .env.example .env
# 编辑 .env 填入实际配置

go run .
```

### 配置说明

| 环境变量 | 必填 | 说明 | 默认值 |
|---------|------|------|--------|
| `FEISHU_APP_ID` | 是 | 飞书应用 App ID | — |
| `FEISHU_APP_SECRET` | 是 | 飞书应用 App Secret | — |
| `LLM_ENDPOINT` | 否 | LLM API 端点 URL | `https://api.moonshot.cn/v1` |
| `LLM_API_KEY` | 是 | LLM API Key | — |
| `LLM_MODEL` | 否 | 模型名称 | `kimi-k2.5` |
| `SQLITE_PATH` | 否 | SQLite 数据库路径 | `./data/okr-agent.db` |
| `OKR_USER_IDS` | 否 | 手动指定用户 ID，逗号分隔 | — |
| `FEISHU_DEPARTMENT_IDS` | 否 | 部门 ID，逗号分隔 | — |
| `CRON_SCHEDULE` | 否 | 主检查 Cron 表达式 | `0 9 * * 1`（每周一 9:00） |
| `HEALTH_PORT` | 否 | 健康检查 HTTP 端口 | `8080` |
| `RISK_DAYS_HIGH` | 否 | 高风险阈值（天） | `14` |
| `RISK_DAYS_CRITICAL` | 否 | 危急风险阈值（天） | `21` |
| `RISK_COOLDOWN_HIGH_HOURS` | 否 | 高风险提醒冷却（小时） | `72` |
| `RISK_COOLDOWN_CRITICAL_HOURS` | 否 | 危急提醒冷却（小时） | `24` |

> `OKR_USER_IDS` 和 `FEISHU_DEPARTMENT_IDS` 至少配置一项。

## Docker 部署

```bash
docker build -t okr-agent .

docker run -d \
  --name okr-agent \
  -v okr-data:/app/data \
  --env-file .env \
  okr-agent
```

`/app/data` 挂载卷用于持久化 SQLite 数据库。

## 构建

```bash
# 本地构建
go build -o okr-agent .

# 交叉编译（零 CGO 依赖）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o okr-agent .
```

## 使用示例

通过飞书私聊 Bot：

```
用户: 帮我看看张三的 OKR
Agent: [调用 list_team_members → get_user_okrs] 返回详细评估

用户: 他上个月情况怎么样？
Agent: [调用 get_okr_history] 基于上下文对比分析

用户: 提醒他更新一下进度
Agent: [调用 send_reminder] 已发送提醒

用户: 团队里还有谁的 OKR 需要关注？
Agent: [调用 list_team_members → 批量 get_user_okrs] 返回风险排名

用户: 帮我处理一下这个文档的评论 https://xxx.feishu.cn/wiki/ABC123
Agent: [调用 list_doc_comments → get_doc_content → list_doc_blocks → update_doc_block → reply_doc_comment] 逐条处理评论
```
