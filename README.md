# OKR Agent

自动化 OKR 检查与评价工具。通过飞书定期获取团队成员的 OKR 数据，使用 Claude AI 进行智能评价，并将结果通过飞书私聊发送给对应成员。

## 功能

- **定时检查** — 通过 Cron 表达式配置，定期自动检查所有监控用户的 OKR 进展
- **AI 评价** — 调用 Claude 对 OKR 的更新频率、KR 进度合理性、SMART 原则符合度进行评价，并给出改进建议
- **飞书 Bot** — 支持 WebSocket 长连接（无需公网地址），通过私聊命令交互：
  - `检查OKR` — 检查所有监控用户的 OKR 并返回摘要
  - `评价 @某人` — 针对指定人进行 OKR 评价
  - `帮助` — 显示可用命令
- **灵活的用户来源** — 支持手动指定用户 ID 和通过部门 ID 动态获取成员，两种方式可同时使用并自动去重

## 项目结构

```
okr-agent/
├── main.go                # 入口：启动 cron + 飞书 bot + 信号处理
├── config/config.go       # 配置加载（.env / 环境变量）
├── feishu/
│   ├── client.go          # 飞书客户端初始化
│   ├── user.go            # 用户管理（静态 ID + 部门 API）
│   ├── okr.go             # OKR API 调用与数据格式化
│   ├── message.go         # 发送私聊消息（文本 / 富文本）
│   └── bot.go             # Bot WebSocket 事件处理
├── evaluator/evaluator.go # Claude AI 评价逻辑
└── scheduler/scheduler.go # Cron 定时任务调度
```

## 快速开始

### 前置条件

- Go 1.21+
- [飞书开放平台](https://open.feishu.cn/)应用（需开通相关权限）
- [Anthropic API Key](https://console.anthropic.com/)

### 飞书应用权限

| 权限 | 用途 |
|------|------|
| `okr:okr:readonly` | 读取用户 OKR 数据 |
| `im:message:send_as_bot` | 以机器人身份发送消息 |
| `im:message.receive_v1` | 接收用户发给机器人的消息 |
| `contact:user.id:readonly` | 读取用户 ID |
| `contact:department.member:readonly` | 通过部门 ID 获取成员列表 |

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
| `ANTHROPIC_API_KEY` | 是 | Anthropic API Key | — |
| `OKR_USER_IDS` | 否 | 手动指定用户 ID，逗号分隔 | — |
| `FEISHU_DEPARTMENT_IDS` | 否 | 部门 ID，逗号分隔，自动获取成员 | — |
| `CRON_SCHEDULE` | 否 | Cron 表达式 | `0 9 * * 1`（每周一 9:00） |

> `OKR_USER_IDS` 和 `FEISHU_DEPARTMENT_IDS` 至少配置一项。

## 工作流程

```
Cron 触发 / Bot 命令
        │
        ▼
  收集用户列表（静态 + 部门 API）
        │
        ▼
  获取每个用户的 OKR 数据（飞书 OKR API）
        │
        ▼
  格式化 OKR 数据 → 发送给 Claude 评价
        │
        ▼
  将评价结果通过飞书私聊发送给用户
```

## 构建

```bash
go build -o okr-agent .
```
