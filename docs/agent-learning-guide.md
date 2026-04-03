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
| **大脑 (LLM)** | 理解意图、规划步骤、生成回复 | Kimi K2.5（兼容 OpenAI 格式的 API） |
| **工具 (Tools)** | 与外部世界交互的能力 | 13 个工具（查 OKR、发消息、文档操作等） |
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

### 2.4 错误即反馈

Agent 的一个核心设计原则：**工具执行的错误不是崩溃，而是反馈给 LLM 的数据**。

看 `agent/agent.go` 中的错误处理：

```go
output, execErr := a.registry.Execute(ctx, tc.Function.Name, json.RawMessage(tc.Function.Arguments))

content := output
if execErr != nil {
    log.Printf("Tool %s error: %v", tc.Function.Name, execErr)
    content = fmt.Sprintf("Error: %s", execErr.Error())
}

// 错误信息作为 tool 角色消息反馈给 LLM
messages = append(messages, llm.Message{
    Role:       "tool",
    ToolCallID: tc.ID,
    Content:    content,  // 可能是正常结果，也可能是 "Error: ..."
})
```

当 LLM 调用了一个不存在的工具名（幻觉），Registry 返回 `"Error: unknown tool: xyz"`。LLM 在下一轮看到这个错误后，可以：

1. **换参数重试**：修正拼写错误的用户 ID
2. **换工具**：发现 `get_user_okr` 不存在，改用 `get_user_okrs`
3. **告知用户**：如果确实无法完成，生成友好的错误提示

这是 Agent 与传统程序的关键差异——传统程序遇到错误就中断，Agent 把错误当作推理的输入。

### 2.5 循环中的防御设计

ReAct 循环有几个容易忽略的防御细节：

**系统消息每轮刷新**：

```go
for i := 0; i < MaxIterations; i++ {
    truncated := truncateHistory(messages, MaxHistoryTurns)
    allMessages := make([]llm.Message, 0, len(truncated)+1)
    allMessages = append(allMessages, llm.Message{Role: "system", Content: systemPrompt})
    allMessages = append(allMessages, truncated...)
    // ...
}
```

系统消息在**每轮迭代**都重新插入到消息列表最前面，而不是只在第一轮插入。原因：经过多轮工具调用和历史截断后，第一轮的系统消息可能已被截掉。每轮刷新保证 LLM 始终知道自己的身份和行为规范。

**finish_reason 的完整处理**：

```go
if choice.FinishReason == "stop" || choice.FinishReason == "length" {
    return &RunResult{Response: extractText(choice.Message), ToolCalls: totalToolCalls}, nil
}
```

- `"stop"`：LLM 自然完成，正常返回
- `"length"`：达到 token 上限被截断——仍然返回已有内容，而不是丢弃
- `"tool_calls"`：继续执行工具
- **未知值**：防御性返回当前内容，防止 API 行为变更导致崩溃

### 2.6 LLM 客户端：Agent 的通信层

`agent/agent.go` 中的 `a.llm.CreateMessage(ctx, req)` 背后是一个完整的 HTTP 通信层。理解它有助于理解 Agent 的"神经系统"。

**请求/响应协议**：

```
Agent 循环                   LLM 客户端                  LLM API (Kimi/OpenAI)
    │                            │                           │
    │  Request{Messages, Tools}  │                           │
    ├───────────────────────────>│  HTTP POST + Bearer Token │
    │                            ├──────────────────────────>│
    │                            │  Response{Choices, Usage}  │
    │                            │<──────────────────────────┤
    │  *Response                 │                           │
    │<───────────────────────────┤                           │
```

**ToolCall ID 机制**：

LLM 在一次响应中可能请求调用多个工具，每个调用都带有唯一 ID：

```go
// LLM 返回的工具调用
type ToolCall struct {
    ID       string       `json:"id"`       // 唯一标识，如 "call_abc123"
    Function FunctionCall `json:"function"` // 函数名 + 参数
}

// 工具结果通过 ToolCallID 关联回对应的调用
messages = append(messages, llm.Message{
    Role:       "tool",
    ToolCallID: tc.ID,    // 必须与 ToolCall.ID 匹配
    Content:    output,
})
```

这个 ID 是 LLM 将多个工具结果与各自请求对应起来的关键。

**LLM 客户端的通用性**（`llm/client.go`）：

```go
// URL 使用标准 OpenAI 兼容格式
url := fmt.Sprintf("%s/chat/completions", c.endpoint)

// 使用标准 Bearer Token 认证
httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

// 支持 context 取消和超时
httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
```

当前默认使用 Kimi K2.5（endpoint: `https://api.moonshot.cn/v1`），但由于采用了标准的 OpenAI 兼容协议，只需修改 `LLM_ENDPOINT`、`LLM_API_KEY` 和 `LLM_MODEL` 三个环境变量即可切换到其他提供商（OpenAI、DeepSeek 等）。

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
┌──────────────────────────────────────────────────────────────────────────────┐
│                              Tool Registry                                    │
├──────────────┬────────────────────┬─────────────────────┬────────────────────┤
│  OKR 查询     │  消息通知            │  团队 & 其他         │  文档操作            │
│              │                    │                     │                    │
│ get_user_okrs│ send_message       │ list_team_members   │ list_doc_comments  │
│ get_okr_hist │ send_reminder      │ update_okr_progress │ get_doc_content    │
│ compare_okr  │ send_team_         │   (桩)              │ list_doc_blocks    │
│   _periods   │   notification     │                     │ update_doc_block   │
│              │                    │                     │ reply_doc_comment  │
└──────────────┴────────────────────┴─────────────────────┴────────────────────┘
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

### 3.5 Schema 预编译与性能

每个工具在构造函数中**预编译** JSON Schema：

```go
func NewGetUserOKRsTool(fc *feishu.Client, store *memory.Store) *GetUserOKRsTool {
    // 在启动阶段一次性编译为 json.RawMessage
    schema, _ := json.Marshal(map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "user_id": map[string]interface{}{
                "type": "string", "description": "用户的 open_id",
            },
            // ...
        },
        "required": []string{"user_id"},
    })
    return &GetUserOKRsTool{feishu: fc, store: store, schema: schema}
}

func (t *GetUserOKRsTool) InputSchema() json.RawMessage { return t.schema }
```

`InputSchema()` 在每次 Agent 循环中都会被调用（通过 `registry.GetToolParams()`），预编译避免了每次调用时重复序列化 map 为 JSON。

### 3.6 桩工具：控制 LLM 幻觉

`update_okr_progress` 是一个特殊的**桩工具**——飞书 OKR API 没有写入接口，所以它实际上只返回操作指引：

```go
func (t *UpdateOKRProgressTool) Description() string {
    return "更新 OKR 进度（注意：飞书 OKR API 暂不支持写操作，此工具会引导用户手动更新）"
}

func (t *UpdateOKRProgressTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
    return "飞书 OKR API 目前不支持通过 API 更新进度。请引导用户在飞书 OKR 页面中手动更新进度。" +
        "\n\n操作步骤：\n1. 打开飞书 → OKR\n2. 找到对应的 KR\n3. 点击进度条更新百分比", nil
}
```

为什么不直接去掉这个工具？三个原因：

1. **防幻觉**：如果注册表里没有"更新进度"的工具，LLM 可能幻觉出一个不存在的 API 调用，或者直接声称"已帮你更新"
2. **引导用户**：Description 中明确标注"暂不支持"，LLM 读到后会生成正确的手动操作指引
3. **未来扩展**：飞书未来开放写入 API 时，只需替换 Execute 实现

### 3.7 消息工具的粒度设计

OKR Agent 有三个消息工具，服务不同场景：

| 工具 | 消息格式 | 参数 | 典型场景 |
|------|---------|------|---------|
| `send_message` | 纯文本 | user_id, text | 日常沟通、回答问题 |
| `send_reminder` | 富文本（带标题） | user_id, title, content | OKR 评估、风险提醒 |
| `send_team_notification` | 纯文本或富文本 | text, title(可选) | 全员广播、周期通知 |

`send_team_notification` 的容错设计值得注意——单个用户发送失败不中断整体：

```go
sent, failed := 0, 0
for _, u := range users {
    if sendErr != nil {
        log.Printf("Failed to notify %s: %v", u.OpenID, sendErr)
        failed++
    } else {
        sent++
    }
}
return fmt.Sprintf("团队通知完成：成功 %d 人，失败 %d 人", sent, failed), nil
```

给 LLM 提供不同粒度的工具，让它根据上下文自主选择：Agent 发送评估报告时选 `send_reminder`（有标题更正式），闲聊回复时选 `send_message`（纯文本更自然）。

---

## 第四章：记忆系统

### 4.1 记忆架构总览

LLM 本身是**无状态**的——每次调用都是全新的，不记得之前的对话。OKR Agent 用 SQLite 实现了三层记忆：

```
┌──────────────────────────────────────────────────────────────────┐
│                       Memory Architecture                        │
├──────────────────┬─────────────────────┬─────────────────────────┤
│ Tier 1: 对话记忆  │ Tier 2: 数据快照     │ Tier 3: 长期记忆         │
│ (Working Memory) │ (Append-Only Log)   │ (Persistent State)      │
├──────────────────┼─────────────────────┼─────────────────────────┤
│ conversations    │ okr_snapshots       │ user_context            │
│                  │ evaluation_history  │ scheduler_state         │
├──────────────────┼─────────────────────┼─────────────────────────┤
│ 24h TTL          │ 永久保留             │ 永久保留                 │
│ 每用户 1 条       │ 追加写入，多条/用户   │ 每用户 1 条，UPSERT      │
│ 读时过期清理      │ 查询时 LIMIT 分页    │ 查询未命中返回默认值       │
├──────────────────┼─────────────────────┼─────────────────────────┤
│ 类比：工作记忆     │ 类比：日记本          │ 类比：身份证 + 病历卡     │
└──────────────────┴─────────────────────┴─────────────────────────┘
```

### 4.2 Tier 1 — 对话记忆（Working Memory）

对话记忆让用户可以多轮追问而不失去上下文：

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

**关键设计决策**：

**懒过期（Lazy Expiry）**：过期的对话不是由后台定时任务清理，而是在**读取时**检查并删除：

```go
func (s *Store) GetConversation(ctx context.Context, userID string) ([]llm.Message, error) {
    // 查询记录...
    if time.Since(updatedAt) > conversationTTL {  // 24h TTL
        s.db.ExecContext(ctx, `DELETE FROM conversations WHERE user_id = ?`, userID)
        return nil, nil  // 返回 nil → 开始新对话
    }
    // 反序列化并返回...
}
```

对于单实例服务，懒过期比后台定时清理更简单——不需要额外的 goroutine 和定时器。

**截断机制**：`truncateHistory(messages, MaxHistoryTurns)` 保留最近 `MaxHistoryTurns × 2` 条消息（每轮对话约产生 2 条消息：user + assistant），防止对话过长超出 LLM 的 token 限制。

**UPSERT 存储**：每个用户始终只有一条对话记录，新消息覆盖旧记录。

### 4.3 Tier 2 — 数据快照（Append-Only Log）

每次调用 `get_user_okrs` 工具时，自动保存一份 OKR 数据快照：

```go
func (t *GetUserOKRsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
    okrData, _ := t.feishu.GetUserOKRs(ctx, params.UserID, params.Month)
    formatted := feishu.FormatOKRForEvaluation(okrData)

    // 自动保存快照——静默失败，不阻塞主流程
    if t.store != nil {
        _ = t.store.SaveOKRSnapshot(ctx, params.UserID, month, formatted)
    }
    return formatted, nil
}
```

与对话记忆的区别：

- **追加写入**：不覆盖旧数据，同一用户同一月份可以有多条快照
- **永久保留**：没有 TTL，积累的快照越多趋势分析越有价值
- **静默失败**：`_ =` 忽略保存错误——快照是增值功能，不能影响主流程

`get_okr_history` 工具从这些快照中读取数据，让 Agent 可以对比"上周和这周进度有什么变化"。

### 4.4 Tier 3 — 用户上下文（Long-Term Memory）

`UserContext` 是跨会话持久化的长期记忆，**不受对话 24h TTL 影响**：

```go
type UserContext struct {
    UserID            string     // 用户标识
    Language          string     // 偏好语言："zh" 或 "en"
    ReminderFrequency string     // 提醒频率："weekly"、"daily"
    AgentNotes        string     // Agent 维护的用户备注（跨会话积累）
    LastInteraction   *time.Time // 最后交互时间
}
```

其中 `AgentNotes` 最有意思——它是一个 Agent 可写的自由文本字段，用于记录对用户的观察（如"该用户经常延迟更新 KR3"）。这些备注会被注入到系统提示词中（第五章详述），让 Agent 在新会话中也能"记住"用户特点。

**降级设计**——查询出错时返回默认值，不中断服务：

```go
func (s *Store) GetUserContext(ctx context.Context, userID string) (*UserContext, error) {
    uc := &UserContext{UserID: userID, Language: "zh", ReminderFrequency: "weekly"}
    // ...
    if err != nil {
        return uc, nil  // 查询出错也返回默认值，保证系统可用
    }
    return uc, nil
}
```

`agent.go` 中的调用方利用了这个设计：`uc, _ := a.store.GetUserContext(ctx, userID)`——忽略错误，因为总会拿到一个可用的 UserContext。

### 4.5 调度状态（Operational Memory）

调度器有自己的状态表 `scheduler_state`，跟踪每个用户的 OKR 更新风险：

```go
type SchedulerState struct {
    UserID          string     // 用户标识
    RiskLevel       string     // 风险等级：normal / high / critical
    DaysSinceUpdate int        // 距上次 OKR 更新的天数
    LastReminder    *time.Time // 上次发送提醒的时间
}
```

调度器通过 `LastReminder` 控制提醒频率——避免每天扫描时重复发送（第六章详述）。同样遵循默认值降级模式：首次扫描时用户没有记录，`GetSchedulerState` 返回 `RiskLevel="normal"` 的默认值。

### 4.6 存储引擎：SQLite 设计选择

```go
func NewStore(dbPath string) (*Store, error) {
    db, _ := sql.Open("sqlite", dbPath)
    db.SetMaxOpenConns(1)                              // 单写避免锁冲突
    db.Exec("PRAGMA journal_mode=WAL")                 // WAL 模式支持并发读
    // ...
}
```

- **WAL 模式** + `MaxOpenConns(1)`：单写多读，避免 `"database is locked"` 错误
- **纯 Go 驱动**（`modernc.org/sqlite`）：零 CGO 依赖，`CGO_ENABLED=0` 交叉编译无障碍
- **幂等迁移**：所有建表语句使用 `IF NOT EXISTS`，重启应用不会出错

---

## 第五章：系统提示词工程

### 5.1 系统提示词的作用

系统提示词定义了 Agent 的**身份、能力边界和行为规范**。它是 Agent 的"岗位说明书"。

### 5.2 动态系统提示词的构建

OKR Agent 的系统提示词不是静态文本，而是由 `BuildSystemPrompt()` 动态组装：

```go
func BuildSystemPrompt(uc *memory.UserContext) string {
    prompt := baseSystemPrompt  // ① 静态基础部分

    // ② 注入当前日期
    prompt += fmt.Sprintf("\n\n## 当前信息\n- 当前日期：%s",
        time.Now().Format("2006-01-02"))

    if uc != nil {
        // ③ 语言路由
        if uc.Language == "en" {
            prompt += "\n- 用户偏好语言：English（请使用英文回复）"
        }
        // ④ 跨会话记忆注入
        if uc.AgentNotes != "" {
            prompt += fmt.Sprintf("\n- 关于该用户的备注：%s", uc.AgentNotes)
        }
        // ⑤ 活跃度感知
        if uc.LastInteraction != nil {
            prompt += fmt.Sprintf("\n- 上次互动：%s",
                uc.LastInteraction.Format("2006-01-02 15:04"))
        }
    }
    return prompt
}
```

分解看各部分的作用：

```
BuildSystemPrompt(uc)
    │
    ├─ baseSystemPrompt（静态，编译时确定）
    │   ├─ 身份定义："你是 OKR 助手"
    │   ├─ 能力列表：查询、分析、发送消息、文档操作...
    │   ├─ 工作方式：先获取数据，再分析...
    │   ├─ 评估标准：SMART 原则，每个 O/KR 单独评价
    │   ├─ 文档评论处理工作流（6 步）
    │   └─ 响应规范：中文、简洁、有价值
    │
    └─ 动态注入（运行时确定）
        ├─ 当前日期 → LLM 据此判断"月度过半进度偏低"
        ├─ 用户语言 → "en" 时切换英文回复
        ├─ Agent 备注 → 跨会话的长期记忆
        └─ 上次互动 → 活跃度上下文
```

### 5.3 动态注入的设计意图

**为什么注入日期**？LLM 自身不知道"今天"是几号。OKR 评估中的"月度已过 80%，进度只有 30%"这类判断，需要 LLM 知道当前日期才能做出。

**为什么注入 AgentNotes**？这是第四章 Tier 3 长期记忆与系统提示词的桥梁。Agent 在一次会话中记录的用户备注，会在下次会话的系统提示词中被 LLM 看到，实现"跨会话记忆"。

**RunOneShot 传 nil 的设计意图**：

```go
func (a *Agent) RunOneShot(ctx context.Context, text string) (*RunResult, error) {
    systemPrompt := BuildSystemPrompt(nil)  // nil → 无用户特定上下文
    // ...
}
```

`RunOneShot` 用于调度器的后台任务（如批量评估、风险提醒），没有"当前用户"的概念。传 nil 让 `BuildSystemPrompt` 只包含静态基础部分和当前日期，不注入任何用户特定信息。调度器会在自己构造的 prompt 中提供目标用户的信息。

### 5.4 好的提示词 vs 差的提示词

```
❌ 差: "你是一个 AI 助手，帮用户看 OKR"

✅ 好: "你是 OKR 助手。当用户请求评估 OKR 时，先使用 get_user_okrs
      工具获取数据。每个 Objective 和 KR 都必须单独评价。进度评价要
      结合当前日期和 OKR 周期判断。"
```

差异在于：
- **具体的操作指引**：告诉 Agent 先做什么、后做什么
- **明确的质量标准**：什么算"好的评价"
- **边界约束**：不要跳过任何 O 或 KR

---

## 第六章：智能调度

### 6.1 三个定时任务

调度器注册了三个 Cron Job：

```
┌────────────────────────────────────────────────────────────┐
│                    Scheduler Cron Jobs                      │
├─────────────┬──────────────────────────────────────────────┤
│ 周一 09:00   │ RunCheck: Agent 驱动的全员 OKR 评估           │
│ (可配置)     │ 对每个用户执行 agent.RunOneShot()              │
│             │ → Agent 自主获取 OKR → 评估 → 发送评价          │
├─────────────┼──────────────────────────────────────────────┤
│ 每天 10:00   │ DailyRiskScan: 风险扫描                      │
│             │ 获取 OKR → 计算未更新天数 → 判断风险等级         │
│             │ → 高风险用户触发 Agent 个性化提醒                │
├─────────────┼──────────────────────────────────────────────┤
│ 周五 10:00   │ SendReminder: 固定文案提醒                    │
│             │ 向全员发送统一的 OKR 更新提醒文本                │
└─────────────┴──────────────────────────────────────────────┘
```

### 6.2 风险分级逻辑

`DailyRiskScan` 的核心是根据 OKR 最后更新时间判断风险等级：

```go
daysSinceUpdate := int(time.Since(time.Unix(lastModified, 0)).Hours() / 24)
if lastModified == 0 {
    daysSinceUpdate = 999  // 从未更新 → 视为最高风险
}
```

```
┌───────────────┬──────────┬──────────────┬─────────────────────┐
│ 未更新天数      │ 风险等级  │ 提醒冷却时间   │ 行为                 │
├───────────────┼──────────┼──────────────┼─────────────────────┤
│ 0-6 天         │ normal   │ —            │ 不主动提醒            │
│ 7-13 天        │ normal   │ —            │ 由周五统一提醒覆盖     │
│ 14-20 天       │ high     │ 3 天          │ Agent 生成个性化提醒  │
│ ≥21 天         │ critical │ 1 天          │ Agent 每日催促       │
│ 从未更新 (999)  │ critical │ 1 天          │ Agent 每日催促       │
└───────────────┴──────────┴──────────────┴─────────────────────┘
```

冷却时间通过 `SchedulerState.LastReminder` 控制：

```go
case "critical":
    if existingState.LastReminder == nil || time.Since(*existingState.LastReminder) > 24*time.Hour {
        shouldRemind = true
    }
case "high":
    if existingState.LastReminder == nil || time.Since(*existingState.LastReminder) > 3*24*time.Hour {
        shouldRemind = true
    }
```

### 6.3 个性化提醒生成

关键区别：固定文案 vs Agent 生成。

```
固定文案 (SendReminder):
    "请记得更新本月的 OKR 进展"           ← 所有人收到一样的

Agent 生成 (DailyRiskScan):
    "Kelvin 你好，你的 KR2（Lightning
    Talk）进度还是 0%，3 月已经过去       ← 根据具体 OKR 内容生成
    83% 了。建议本周安排一次团队分享
    来推进这个 KR。"
```

Agent 个性化提醒的实现——通过 `RunOneShot()` 启动一个临时 Agent：

```go
prompt := fmt.Sprintf(
    "用户 %s (open_id: %s) 已经 %d 天没有更新 OKR。风险等级：%s。"+
    "请先查看该用户的 OKR 数据，然后生成一条个性化的提醒消息并发送给该用户。"+
    "提醒应当友好但有紧迫感，提及具体的 OKR 内容。",
    name, u.OpenID, daysSinceUpdate, riskLevel)

result, err := s.agent.RunOneShot(ctx, prompt)
```

调度器不自己写提醒文案——它只构造一个描述情况的 prompt，然后让 Agent 自主完成整个流程：调用 `get_user_okrs` 获取数据 → 分析具体 OKR → 调用 `send_reminder` 发送个性化消息。

### 6.4 批量容错

调度器处理多个用户时，单个用户失败不中断整体流程：

```go
for _, u := range users {
    result, err := s.agent.RunOneShot(ctx, prompt)
    if err != nil {
        log.Printf("Agent error for %s: %v", name, err)
        continue  // 记录错误，继续处理下一个用户
    }
}
```

状态保存同样容错：`_ = s.store.SaveSchedulerState(ctx, state)`——即使持久化失败，不影响本轮扫描的其他用户。下次扫描时会重新计算状态。

---

## 第七章：工程实践

### 7.1 防护措施

Agent 是自主的，但不能失控：

| 风险 | 防护 | 代码位置 |
|------|------|---------|
| 无限循环 | `MaxIterations = 10` | `agent/agent.go` |
| Token 爆炸 | `MaxHistoryTurns = 20`，截断历史 | `agent/agent.go` |
| 工具滥用 | 桩工具（如 update_okr_progress） | `tools/okr_ops.go` |
| 重复处理 | 消息去重（event_id + 定期清理） | `feishu/bot.go` |
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

注意工具命名使用 `snake_case`（如 `get_user_okrs`、`send_message`）——LLM 的训练数据中函数调用普遍使用此格式，snake_case 能提高工具选择准确率。

### 7.5 优雅降级模式

Agent 的用户无法调试中间错误——他们看到"出错了"就没有下文了。因此系统要尽量自愈。OKR Agent 中有三种降级模式：

**模式 1：查询出错返回默认值**

```go
// GetUserContext 和 GetSchedulerState 查询出错时返回默认值
if err != nil {
    return uc, nil  // 不返回错误，返回默认语言=zh、频率=weekly 的 UserContext
}
```

调用方可以无条件使用返回值：`uc, _ := a.store.GetUserContext(ctx, userID)`。

**模式 2：后台写入静默失败**

```go
// OKR 快照保存失败不影响工具返回数据
_ = t.store.SaveOKRSnapshot(ctx, params.UserID, month, formatted)

// 交互时间更新失败不影响 Agent 回复
_ = a.store.TouchUserInteraction(ctx, userID)
```

**模式 3：批量处理逐个容错**

```go
// 调度器、团队通知：单个用户失败继续处理下一个
for _, u := range users {
    if err := process(u); err != nil {
        log.Printf("Error for %s: %v", u.Name, err)
        continue
    }
}
```

### 7.6 并发安全

**消息去重**：飞书 Bot 可能收到重复事件，用 `sync.Mutex` 保护去重 map：

```go
b.seenMu.Lock()
if _, dup := b.seen[eventID]; dup {
    b.seenMu.Unlock()
    return  // 跳过已处理的事件
}
b.seen[eventID] = time.Now()
b.seenMu.Unlock()
```

`seen` map 通过后台 `cleanupLoop` 定期清理（每小时清理 2 小时前的记录），防止内存泄漏。

**SQLite 单写**：`db.SetMaxOpenConns(1)` 确保同一时间只有一个连接，避免 `"database is locked"` 错误。WAL 模式下读操作不受此限制。

**Context 链路**：从 Bot 消息处理到 LLM API 调用，`context.Context` 一路传递，支持超时和取消。

---

## 第八章：核心概念速查

| 概念 | 一句话解释 |
|------|-----------|
| **Agent** | LLM + 工具 + 记忆 + 自主决策循环 |
| **ReAct** | 思考→行动→观察→再思考的循环模式 |
| **Tool Calling** | LLM 表达"我想调用某个函数"，代码负责执行 |
| **System Prompt** | Agent 的岗位说明书，定义身份、能力和规范 |
| **Conversation Memory** | 短期记忆（Tier 1），让多轮对话有上下文 |
| **Snapshot** | 自动保存的数据快照（Tier 2），支持趋势分析 |
| **UserContext** | 长期记忆（Tier 3），跨会话持久化用户偏好和 Agent 备注 |
| **Registry Pattern** | 工具统一注册和分发，解耦 Agent 和工具实现 |
| **RunOneShot** | 无状态的单次 Agent 运行，适用于后台任务 |
| **Risk Escalation** | 基于数据信号动态升级响应策略（normal→high→critical） |
| **Max Iterations** | 防止 Agent 无限循环的安全阀 |
| **Lazy Expiry** | 读时检查过期并清理，免去后台清理任务 |
| **Graceful Degradation** | 查询返回默认值、写入静默失败、批量逐个容错 |
| **ToolCall ID** | 唯一 ID 将工具结果关联回对应的调用请求 |

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

## 附录 A：OKR Agent 数据流全景

```
飞书用户发消息
    │
    ▼
feishu/bot.go ── event_id 去重（mutex）── 提取 @mention ── 富化文本
    │
    ▼
agent.Run(userID, text)
    │
    ├─ memory: 加载对话历史 (24h TTL, 懒过期)
    ├─ memory: 加载用户偏好 (UserContext, 降级返回默认值)
    ├─ BuildSystemPrompt(uc): 静态基础 + 日期 + 语言 + 备注 + 互动时间
    │
    ▼
ReAct 循环 (最多 10 轮)
    │
    ├─ 每轮重新注入 system message（防截断丢失）
    ├─ 调用 LLM (带 13 个工具定义 + ToolCall ID)
    │
    ├─ finish_reason == "tool_calls"?
    │       │
    │       ├─ tools/registry: 按 name 分发执行
    │       ├─ feishu/*: 调用飞书 API (OKR/消息/用户/文档)
    │       ├─ memory: 自动保存 OKR 快照（静默失败）
    │       ├─ 执行成功 → 结果追加到对话 → 继续循环
    │       └─ 执行失败 → "Error: ..." 追加到对话 → LLM 自修复
    │
    ├─ finish_reason == "stop" / "length"?
    │       │
    │       ├─ memory: 保存对话历史 (UPSERT)
    │       ├─ memory: 更新用户交互时间（静默失败）
    │       └─ 返回文本 → feishu 发送给用户
    │
    └─ finish_reason == 未知?
            └─ 防御性返回当前内容

定时任务 (scheduler)
    │
    ├─ 周一 09:00: agent.RunOneShot(nil) × N 用户 → 批量评估
    ├─ 每天 10:00: 风险扫描 → 分级 (normal/high/critical)
    │              → 检查冷却时间 → agent.RunOneShot(nil) → 个性化提醒
    └─ 周五 10:00: 固定文案 → feishu.SendTextMessage() × N 用户
```

## 附录 B：平台集成笔记（飞书）

本项目使用飞书作为消息平台，但涉及的集成模式适用于任何聊天平台（Slack、Teams、Discord 等）。

### 入口适配器

Bot 通过 WebSocket 接收事件，将消息路由到 Agent：

```go
bot := feishu.NewBot(feishuClient, func(ctx context.Context, senderID string, _ []feishu.MentionedUser, text string) string {
    result, _ := ag.Run(ctx, senderID, text)
    return result.Response
})
go bot.Start()
```

`NewBot` 的 handler 参数是平台层与 Agent 层的桥梁——任何能提供 `(senderID, text) → response` 回调的消息平台都能接入。

### @mention 处理

飞书消息中的 `@_user_1` 标记会被清理，同时被提及用户的信息以结构化格式附加到文本末尾：

```go
text = cleanMentions(text)  // 移除 @_user_N 标记

enrichedText = text + "\n\n[提及的用户: Kelvin (open_id: ou_xxx)]"
```

这让 Agent 能识别"帮我看看 @Kelvin 的 OKR"中的 Kelvin 是谁，并获得其 open_id 以调用工具。

### 事件去重

飞书（以及大多数消息平台）可能因网络重试发送重复事件。Bot 用 `event_id` + `sync.Mutex` 去重：

```go
b.seenMu.Lock()
if b.seen[eventID] { b.seenMu.Unlock(); return }
b.seen[eventID] = true
b.seenMu.Unlock()
```

### 用户收集与去重

配置支持同时指定 `OKR_USER_IDS`（静态 ID）和 `FEISHU_DEPARTMENT_IDS`（部门，递归展开），`CollectUsers` 自动去重：

```go
seen := make(map[string]bool)
for _, id := range userIDs {
    if !seen[id] { users = append(users, ...); seen[id] = true }
}
for _, dept := range departmentIDs {
    members := getDepartmentMembers(dept)  // 递归 + 分页
    for _, m := range members {
        if !seen[m.OpenID] { users = append(users, m); seen[m.OpenID] = true }
    }
}
```

如果换成 Slack 或 Teams，同样需要实现等效的"用户收集 + 去重"逻辑，只是 API 不同。
