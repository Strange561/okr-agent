package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"okr-agent/feishu"
)

// ================================================================================
// send_message 工具 —— 发送文本消息
//
// 这是 Agent 与用户沟通的最基本工具。Agent 可以通过此工具
// 向指定用户发送纯文本消息。通常用于回答用户的问题或发送简短通知。
// ================================================================================

// SendMessageTool 实现向用户发送文本消息的工具。
//
// 通过飞书的消息 API 向指定用户发送私聊消息。
// 这是 Agent 主动与用户通信的能力之一。
type SendMessageTool struct {
	feishu *feishu.Client  // 飞书客户端，用于发送消息
	schema json.RawMessage // 预编译的 JSON Schema
}

// NewSendMessageTool 创建发送消息工具的实例。
//
// Schema 定义了两个必需参数：
//   - user_id：接收消息的用户 open_id
//   - text：要发送的消息文本
func NewSendMessageTool(fc *feishu.Client) *SendMessageTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user_id": map[string]interface{}{
				"type":        "string",
				"description": "接收消息的用户 open_id",
			},
			"text": map[string]interface{}{
				"type":        "string",
				"description": "要发送的消息文本",
			},
		},
		"required": []string{"user_id", "text"},
	})
	return &SendMessageTool{feishu: fc, schema: schema}
}

func (t *SendMessageTool) Name() string              { return "send_message" }
func (t *SendMessageTool) Description() string        { return "向指定用户发送文本消息" }
func (t *SendMessageTool) InputSchema() json.RawMessage { return t.schema }

// Execute 执行发送文本消息的操作。
//
// 解析参数后调用飞书客户端发送消息。
// 成功时返回确认信息，失败时返回错误。
func (t *SendMessageTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		UserID string `json:"user_id"` // 接收者的 open_id
		Text   string `json:"text"`    // 消息内容
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	// 通过飞书 API 发送纯文本消息
	if err := t.feishu.SendTextMessage(ctx, params.UserID, params.Text); err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return fmt.Sprintf("消息已发送给用户 %s", params.UserID), nil
}

// ================================================================================
// send_reminder 工具 —— 发送富文本提醒
//
// 与 send_message 不同，此工具发送的是带标题的富文本（post）消息，
// 视觉上更醒目，适合用于 OKR 评估结果、更新提醒等重要通知。
// ================================================================================

// SendReminderTool 实现向用户发送富文本提醒消息的工具。
//
// 飞书的 post 消息格式支持标题和富文本内容，
// 比纯文本消息更适合展示结构化的评估结果。
type SendReminderTool struct {
	feishu *feishu.Client  // 飞书客户端
	schema json.RawMessage // 预编译的 JSON Schema
}

// NewSendReminderTool 创建发送提醒工具的实例。
//
// Schema 定义了三个必需参数：
//   - user_id：接收提醒的用户 open_id
//   - title：提醒标题（会在飞书消息中高亮显示）
//   - content：提醒内容
func NewSendReminderTool(fc *feishu.Client) *SendReminderTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user_id": map[string]interface{}{
				"type":        "string",
				"description": "接收提醒的用户 open_id",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "提醒标题",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "提醒内容",
			},
		},
		"required": []string{"user_id", "title", "content"},
	})
	return &SendReminderTool{feishu: fc, schema: schema}
}

func (t *SendReminderTool) Name() string              { return "send_reminder" }
func (t *SendReminderTool) Description() string        { return "向指定用户发送富文本提醒消息" }
func (t *SendReminderTool) InputSchema() json.RawMessage { return t.schema }

// Execute 执行发送富文本提醒的操作。
//
// 解析参数后通过飞书 API 发送 post 格式的消息。
// post 消息有标题，适合用于 OKR 周报、风险提醒等场景。
func (t *SendReminderTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		UserID  string `json:"user_id"` // 接收者的 open_id
		Title   string `json:"title"`   // 提醒标题
		Content string `json:"content"` // 提醒内容
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	// 通过飞书 API 发送富文本消息（post 格式，带标题）
	if err := t.feishu.SendRichMessage(ctx, params.UserID, params.Title, params.Content); err != nil {
		return "", fmt.Errorf("send reminder: %w", err)
	}
	return fmt.Sprintf("提醒已发送给用户 %s", params.UserID), nil
}

// ================================================================================
// send_team_notification 工具 —— 向团队广播通知
//
// 此工具用于向所有配置的团队成员发送通知消息。
// 典型场景：OKR 周期开始通知、团队 OKR 总结报告等。
// 会自动从配置的 user_ids 和 department_ids 中收集所有用户。
// ================================================================================

// SendTeamNotificationTool 实现向团队所有成员广播消息的工具。
//
// 与针对单个用户的 send_message 和 send_reminder 不同，
// 此工具会遍历所有团队成员逐一发送消息。
// 团队成员来源于配置文件中的 userIDs 和 departmentIDs。
type SendTeamNotificationTool struct {
	feishu        *feishu.Client  // 飞书客户端
	userIDs       []string        // 配置的静态用户 ID 列表
	departmentIDs []string        // 配置的部门 ID 列表，会递归获取部门下的所有用户
	schema        json.RawMessage // 预编译的 JSON Schema
}

// NewSendTeamNotificationTool 创建团队通知工具的实例。
//
// 接收静态用户 ID 列表和部门 ID 列表，用于在执行时收集所有需要通知的用户。
// Schema 定义了：
//   - text（必需）：通知内容
//   - title（可选）：通知标题，如果提供则发送富文本消息
func NewSendTeamNotificationTool(fc *feishu.Client, userIDs, departmentIDs []string) *SendTeamNotificationTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{
				"type":        "string",
				"description": "通知内容",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "可选的通知标题（如提供则发送富文本消息）",
			},
		},
		"required": []string{"text"},
	})
	return &SendTeamNotificationTool{feishu: fc, userIDs: userIDs, departmentIDs: departmentIDs, schema: schema}
}

func (t *SendTeamNotificationTool) Name() string              { return "send_team_notification" }
func (t *SendTeamNotificationTool) Description() string        { return "向所有团队成员广播通知消息" }
func (t *SendTeamNotificationTool) InputSchema() json.RawMessage { return t.schema }

// Execute 执行团队广播通知操作。
//
// 工作流程：
//  1. 解析通知内容和可选标题
//  2. 收集所有团队成员（合并静态 ID 和部门用户，自动去重）
//  3. 遍历每个用户逐一发送消息
//  4. 统计成功和失败的数量并返回摘要
//
// 注意：单个用户发送失败不会中断整体流程，而是记录日志后继续。
func (t *SendTeamNotificationTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Text  string `json:"text"`  // 通知内容
		Title string `json:"title"` // 可选的标题
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	// 收集所有团队成员（合并配置的 userIDs 和 departmentIDs 中的用户）
	users, err := t.feishu.CollectUsers(ctx, t.userIDs, t.departmentIDs)
	if err != nil {
		return "", fmt.Errorf("collect users: %w", err)
	}

	// 遍历所有用户逐一发送消息，统计成功和失败数
	sent, failed := 0, 0
	for _, u := range users {
		var sendErr error
		if params.Title != "" {
			// 有标题时发送富文本消息（post 格式）
			sendErr = t.feishu.SendRichMessage(ctx, u.OpenID, params.Title, params.Text)
		} else {
			// 无标题时发送纯文本消息
			sendErr = t.feishu.SendTextMessage(ctx, u.OpenID, params.Text)
		}
		if sendErr != nil {
			log.Printf("Failed to notify %s: %v", u.OpenID, sendErr)
			failed++
		} else {
			sent++
		}
	}

	return fmt.Sprintf("团队通知完成：成功 %d 人，失败 %d 人", sent, failed), nil
}
