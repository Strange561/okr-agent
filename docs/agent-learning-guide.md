# AI Agent 学习指南

> 从 OKR Agent 项目出发，理解 Agent 的核心概念、架构模式和工程实践。

---

## 第一章：什么是 Agent

### 1.1 从自动化到自主化

传统程序和 Agent 的根本区别在于**谁在做决策**：

```
传统程序:  人类设计控制流 → 代码按流程执行 → LLM 只做文本生成
Agent:     人类定义目标和工具 → LLM 自主规划和执行 → 循环直到完成
```

一个类比：

- **传统程序**像流水线工人——按 SOP 操作，遇到 SOP 没覆盖的情况就卡住
- **Agent**像实习生——你告诉他目标，给他权限（工具），他自己想办法完成，遇到问题会调整策略

### 1.2 Agent 的三个核心要素

```
┌─────────────────────────────────────────┐
│                 Agent                    │
│                                         │
│   ┌───────────┐  ┌───────┐  ┌───────┐  │
│   │   大脑     │  │ 工具   │  │ 记忆   │  │
│   │  (LLM)    │  │(Tools) │  │(Memory)│  │
│   └───────────┘  └───────┘  └───────┘  │
│                                         │
│   决策和推理      执行动作     持久化状态  │
└─────────────────────────────────────────┘
```

| 要素 | 作用 | OKR Agent 中的实现 |
|------|------|-------------------|
| **大脑 (LLM)** | 理解意图、规划步骤、生成回复 | Azure OpenAI (gpt-4o) |
| **工具 (Tools)** | 与外部世界交互的能力 | 8 个工具（查 OKR、发消息等） |
| **记忆 (Memory)** | 记住历史和上下文 | SQLite（对话、快照、偏好） |

### 1.3 Agent vs Chatbot vs RAG

| 维度 | Chatbot | RAG | Agent |
|------|---------|-----|-------|
| 能力 | 纯对话 | 对话 + 检索 | 对话 + 推理 + 行动 |
| 外部交互 | 无 | 只读（检索文档） | 读写（调用任意 API） |
| 决策 | 无 | 无（只决定检索什么） | 自主决定做什么 |
| 循环 | 单轮 | 单轮 | **多轮**（思考→行动→观察→再思考） |
| 典型场景 | 客服问答 | 知识库问答 | 自动化工作流、智能助手 |

---

## 第二章：ReAct 模式

### 2.1 什么是 ReAct

ReAct = **Re**asoning + **Act**ing，是目前最主流的 Agent 设计模式。

核心思想：LLM 在每一步**先推理（Reason），再行动（Act），然后观察结果（Observe）**，循环往复直到任务完成。

```
用户: "帮我看看 Kelvin 的 OKR"

循环 1:
  Reason: 用户想查看 Kelvin 的 OKR，但我不知道 Kelvin 的 user_id，先查团队成员
  Act:    调用 list_team_members()
  Observe: 返回 19 人名单，Kelvin 的 open_id 是 ou_xxx

循环 2:
  Reason: 找到了 Kelvin，现在获取他的 OKR 数据，同时查历史做对比
  Act:    调用 get_user_okrs(ou_xxx) + get_okr_history(ou_xxx)
  Observe: 返回 3 月 OKR 数据（2 个 O，4 个 KR）和历史快照

循环 3:
  Reason: 数据齐全了，可以生成评估报告
  Act:    生成最终回复（finish_reason=stop）
  输出:   详细的 OKR 评估报告
```

### 2.2 ReAct 循环的代码实现

OKR Agent 的核心循环在 `agent/agent.go` 中：

```go
for i := 0; i < MaxIterations; i++ {
    // 1. 调用 LLM（带工具定义）
    resp, err := a.llm.CreateMessage(ctx, req)

    // 2. 检查 LLM 的决策
    switch choice.FinishReason {
    case "stop":
        // LLM 认为任务完成，返回最终回复
        return result

    case "tool_calls":
        // LLM 决定调用工具
        for _, tc := range choice.Message.ToolCalls {
            // 3. 执行工具
            output := registry.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
            // 4. 将结果反馈给 LLM（作为下一轮的输入）
            messages = append(messages, toolResultMessage)
        }
        // 继续循环 → LLM 基于工具结果再次推理
    }
}
```

关键设计：
- **`MaxIterations = 10`**：防止 LLM 陷入无限循环
- **工具结果即时反馈**：每次工具执行后，结果追加到对话历史，LLM 在下一轮能看到
- **LLM 掌握控制权**：代码不决定调什么工具、调几次——LLM 自己决定

### 2.3 为什么不用 Chain / Pipeline

对比三种模式：

```
Chain（链式）:     步骤A → 步骤B → 步骤C → 输出
                   ❌ 固定顺序，无法应对变化

Pipeline（管道）:  输入 → [分类] → [处理A|处理B] → [后处理] → 输出
                   ❌ 分支有限，人工预设

ReAct（循环）:     输入 → [思考→行动→观察] × N → 输出
                   ✅ 动态规划，自适应
```

ReAct 的优势：**面对未预见的情况也能工作**。比如用户说"帮我看看张三的 OKR"，如果团队里没有张三，Agent 会自己发现这个问题并告知用户，而不需要你提前写 `if user not found` 的逻辑。

---

## 第三章：Tool Calling（函数调用）

### 3.1 工作原理

Tool Calling 是 Agent 与外部世界交互的桥梁。LLM 本身无法访问数据库、调用 API——它只能输出文本。Tool Calling 的机制是：

```
你告诉 LLM:  "你可以使用这些工具：[工具定义]"
LLM 回复:    "我想调用 get_user_okrs，参数是 {user_id: 'ou_xxx'}"
你的代码:     执行这个函数，把结果返回给 LLM
LLM 继续:    基于结果生成回复
```

LLM 不执行代码——它只**表达意图**，你的代码负责**执行**。

### 3.2 工具定义格式

每个工具需要告诉 LLM 三件事：**名字、功能描述、参数格式**。

以 `get_user_okrs` 为例：

```json
{
  "type": "function",
  "function": {
    "name": "get_user_okrs",
    "description": "获取指定用户的 OKR 数据，包括 Objective、KR、进度和更新时间",
    "parameters": {
      "type": "object",
      "properties": {
        "user_id": {
          "type": "string",
          "description": "用户的 open_id"
        },
        "month": {
          "type": "string",
          "description": "目标月份，格式 YYYY-MM，留空表示当前月"
        }
      },
      "required": ["user_id"]
    }
  }
}
```

工具描述的质量直接影响 Agent 的表现：
- **描述要清晰**：LLM 靠描述理解什么时候该用这个工具
- **参数要有说明**：LLM 靠参数描述知道传什么值
- **必填字段要标注**：`required` 告诉 LLM 哪些不能省略

### 3.3 OKR Agent 的工具体系

```
┌─────────────────────────────────────────────────┐
│                  Tool Registry                   │
├──────────────┬──────────────┬───────────────────┤
│  OKR 查询     │  消息通知     │  团队 & 其他       │
│              │              │                   │
│ get_user_okrs│ send_message │ list_team_members │
│ get_okr_hist │ send_reminder│ update_okr_progress│
│ compare_okr  │ send_team_   │   (桩)             │
│   _periods   │  notification│                   │
└──────────────┴──────────────┴───────────────────┘
```

### 3.4 Tool 接口设计

OKR Agent 用 Go 接口抽象工具：

```go
type Tool interface {
    Name() string                                          // 工具名
    Description() string                                   // 功能描述
    InputSchema() json.RawMessage                          // JSON Schema
    Execute(ctx context.Context, input json.RawMessage) (string, error)  // 执行
}
```

**Registry 模式**：所有工具注册到一个中心 Registry，Agent 循环时通过 name 分发执行。这种设计的好处是添加新工具不需要改 Agent 代码——只需实现接口并注册。

```go
registry := tools.NewRegistry()
registry.Register(tools.NewGetUserOKRsTool(feishuClient, store))
registry.Register(tools.NewSendMessageTool(feishuClient))
// 添加新工具只需要一行
```

---

## 第四章：记忆系统

### 4.1 为什么需要记忆

LLM 本身是**无状态**的——每次调用都是全新的，不记得之前的对话。Agent 需要记忆来实现：

| 记忆类型 | 作用 | 类比 |
|---------|------|------|
| **对话历史** | 多轮对话的上下文 | 短期记忆（工作记忆） |
| **OKR 快照** | 趋势分析和对比 | 长期记忆（事实记忆） |
| **用户偏好** | 个性化响应 | 长期记忆（偏好记忆） |
| **评估记录** | 避免重复工作 | 长期记忆（经验记忆） |

### 4.2 对话记忆的实现

```
用户: "帮我看看 Kelvin 的 OKR"        ← 第一轮
Agent: [评估报告...]

用户: "他上个月怎么样？"               ← 第二轮，Agent 知道"他"指 Kelvin
Agent: [调用 get_okr_history] 对比分析

用户: "提醒他更新"                    ← 第三轮，Agent 仍然知道是 Kelvin
Agent: [调用 send_reminder] 已发送
```

实现方式：

```go
// 加载历史对话（24h 内有效）
conversation, _ := store.GetConversation(ctx, userID)

// 追加新消息
conversation = append(conversation, userMessage)

// 调用 LLM 时带上完整历史
req := llm.Request{Messages: conversation}

// 保存更新后的对话
store.SaveConversation(ctx, userID, conversation)
```

关键设计决策：
- **24 小时过期**：避免过时上下文干扰新对话
- **20 轮截断**：防止超出 LLM 的 token 限制
- **按 user_id 隔离**：每个用户独立的对话空间

### 4.3 自动快照

每次调用 `get_user_okrs` 工具时，自动保存一份快照到数据库：

```go
func (t *GetUserOKRsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
    okrData, _ := t.feishu.GetUserOKRs(ctx, params.UserID, params.Month)
    formatted := feishu.FormatOKRForEvaluation(okrData)

    // 自动保存快照——用户无感知
    t.store.SaveOKRSnapshot(ctx, params.UserID, month, formatted)

    return formatted, nil
}
```

这些快照积累后，`get_okr_history` 工具可以用来做趋势分析，而无需用户手动记录。

---

## 第五章：系统提示词工程

### 5.1 系统提示词的作用

系统提示词定义了 Agent 的**身份、能力边界和行为规范**。它是 Agent 的"岗位说明书"。

### 5.2 OKR Agent 的系统提示词结构

```
┌────────────────────────────────────┐
│           系统提示词                 │
├────────────────────────────────────┤
│ 1. 身份定义                         │
│    "你是 OKR 教练助手"              │
├────────────────────────────────────┤
│ 2. 能力列表                         │
│    "你可以查询 OKR、发送消息..."     │
├────────────────────────────────────┤
│ 3. 工作方式                         │
│    "先获取数据，再进行分析..."       │
├────────────────────────────────────┤
│ 4. 评估标准                         │
│    "每个 O 和 KR 都要评价..."       │
├────────────────────────────────────┤
│ 5. 动态上下文                       │
│    "当前日期: 2026-03-26"           │
│    "用户偏好: 中文"                 │
│    "上次互动: 2026-03-25"           │
└────────────────────────────────────┘
```

### 5.3 好的提示词 vs 差的提示词

```
❌ 差: "你是一个 AI 助手，帮用户看 OKR"

✅ 好: "你是 OKR 教练助手。当用户请求评估 OKR 时，先使用 get_user_okrs
      工具获取数据。每个 Objective 和 KR 都必须单独评价。进度评价要
      结合当前日期和 OKR 周期判断。"
```

差异在于：
- **具体的操作指引**：告诉 Agent 先做什么、后做什么
- **明确的质量标准**：什么算"好的评价"
- **边界约束**：不要跳过任何 O 或 KR

---

## 第六章：智能调度

### 6.1 从固定 Cron 到风险驱动

传统方式：每周一 9:00 检查所有人——**不管有没有问题，统一处理**。

Agent 方式：每天扫描风险，按需触发——**有问题的人多关注，没问题的不打扰**。

```
每日 10:00 风险扫描:
    ┌─ 7 天未更新 → normal  → 周五统一提醒覆盖
    ├─ 14 天未更新 → high   → 3 天内没提醒过？→ Agent 生成个性化提醒
    └─ 21 天未更新 → critical → 每天检查 → Agent 生成升级通知
```

### 6.2 个性化提醒

关键区别：固定模板 vs Agent 生成。

```
固定模板: "请更新你的 OKR。"                  ← 所有人收到一样的

Agent:    "Kelvin 你好，你的 KR2（Lightning
          Talk）进度还是 0%，3 月已经过去      ← 根据具体 OKR 内容生成
          83% 了。建议本周安排一次团队分享
          来推进这个 KR。"
```

实现方式是调用 `agent.RunOneShot()`——启动一个不保存对话的临时 Agent，让它查数据、生成提醒、发送消息，全程自主完成。

---

## 第七章：工程实践

### 7.1 防护措施

Agent 是自主的，但不能失控：

| 风险 | 防护 | 代码位置 |
|------|------|---------|
| 无限循环 | `MaxIterations = 10` | `agent/agent.go` |
| Token 爆炸 | `MaxHistoryTurns = 20`，截断历史 | `agent/agent.go` |
| 工具滥用 | 桩工具（如 update_okr_progress） | `tools/okr_ops.go` |
| 重复处理 | 消息去重（event_id） | `feishu/bot.go` |
| 提醒轰炸 | 检查 last_reminder 时间间隔 | `scheduler/scheduler.go` |

### 7.2 可观测性

每一步都有日志，便于调试：

```
17:08:19 Received message: '帮我看看 Kelvin的okr'
17:08:22 LLM response: finish_reason=tool_calls, tool_calls=1
17:08:22 Executing tool: list_team_members
17:08:29 LLM response: finish_reason=tool_calls, tool_calls=2
17:08:29 Executing tool: get_user_okrs
17:08:30 Executing tool: get_okr_history
17:09:17 LLM response: finish_reason=stop, usage={Prompt:3001, Completion:2059}
```

从日志可以看到 Agent 的完整决策链：3 轮循环、4 次工具调用、5060 tokens。

### 7.3 依赖注入

工具不直接依赖全局变量，而是通过构造函数注入依赖：

```go
// 每个工具声明自己需要的依赖
func NewGetUserOKRsTool(fc *feishu.Client, store *memory.Store) *GetUserOKRsTool

// main.go 中组装
registry.Register(tools.NewGetUserOKRsTool(feishuClient, store))
```

好处：
- **可测试**：可以传入 mock 对象
- **可替换**：换飞书 client 不影响工具逻辑
- **依赖清晰**：一眼看出每个工具需要什么

### 7.4 添加新工具的步骤

只需三步：

**第一步**：在 `tools/` 下新建文件，实现 Tool 接口

```go
type MyNewTool struct {
    schema json.RawMessage
}

func (t *MyNewTool) Name() string              { return "my_new_tool" }
func (t *MyNewTool) Description() string       { return "做某件事" }
func (t *MyNewTool) InputSchema() json.RawMessage { return t.schema }
func (t *MyNewTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
    // 实现逻辑
}
```

**第二步**：在 `main.go` 注册

```go
registry.Register(tools.NewMyNewTool())
```

**第三步**：不需要了。Agent 会自动感知新工具并在合适时调用。

---

## 第八章：核心概念速查

| 概念 | 一句话解释 |
|------|-----------|
| **Agent** | LLM + 工具 + 记忆 + 自主决策循环 |
| **ReAct** | 思考→行动→观察→再思考的循环模式 |
| **Tool Calling** | LLM 表达"我想调用某个函数"，代码负责执行 |
| **System Prompt** | Agent 的岗位说明书，定义身份、能力和规范 |
| **Conversation Memory** | 短期记忆，让多轮对话有上下文 |
| **Snapshot** | 自动保存的数据快照，支持趋势分析 |
| **Registry Pattern** | 工具统一注册和分发，解耦 Agent 和工具实现 |
| **RunOneShot** | 无状态的单次 Agent 运行，适用于后台任务 |
| **Risk Escalation** | 基于数据信号动态升级响应策略 |
| **Max Iterations** | 防止 Agent 无限循环的安全阀 |

---

## 第九章：延伸阅读

### 论文
- [ReAct: Synergizing Reasoning and Acting in Language Models](https://arxiv.org/abs/2210.03629) — ReAct 模式的原始论文
- [Toolformer](https://arxiv.org/abs/2302.04761) — LLM 学习使用工具的研究
- [Reflexion](https://arxiv.org/abs/2303.11366) — Agent 从失败中学习

### 框架对比
| 框架 | 语言 | 特点 |
|------|------|------|
| LangChain | Python | 生态最全，概念较多 |
| LangGraph | Python | 基于图的 Agent 工作流 |
| CrewAI | Python | 多 Agent 协作 |
| Anthropic Agent SDK | Python | 官方出品，简洁 |
| **本项目** | **Go** | **最小实现，无框架依赖** |

本项目的价值在于：**没有使用任何 Agent 框架**，从零实现了完整的 ReAct 循环。核心代码只有 `agent/agent.go` 的 ~140 行。理解了这 140 行，就理解了所有 Agent 框架的核心。

---

## 附录：OKR Agent 数据流全景

```
飞书用户发消息
    │
    ▼
feishu/bot.go ── 去重 + 提取 @mention ── 富化文本
    │
    ▼
agent.Run(userID, text)
    │
    ├─ memory: 加载对话历史 (24h TTL)
    ├─ memory: 加载用户偏好 → 注入系统提示词
    │
    ▼
ReAct 循环 (最多 10 轮)
    │
    ├─ 调用 Azure OpenAI (带 8 个工具定义)
    │
    ├─ finish_reason == "tool_calls"?
    │       │
    │       ├─ tools/registry: 按 name 分发执行
    │       ├─ feishu/*: 调用飞书 API (OKR/消息/用户)
    │       ├─ memory: 自动保存 OKR 快照
    │       └─ 结果追加到对话 → 继续循环
    │
    └─ finish_reason == "stop"?
            │
            ├─ memory: 保存对话历史
            ├─ memory: 更新用户交互时间
            └─ 返回文本 → feishu 发送给用户

定时任务 (scheduler)
    │
    ├─ 周一 09:00: agent.RunOneShot() × N 用户 → 批量评估
    ├─ 每天 10:00: 风险扫描 → 高风险用户 → agent.RunOneShot() → 个性化提醒
    └─ 周五 10:00: 固定文案 → feishu.SendTextMessage() × N 用户
```
