package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"okr-agent/feishu"
)

// --- send_message ---

type SendMessageTool struct {
	feishu *feishu.Client
	schema json.RawMessage
}

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

func (t *SendMessageTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		UserID string `json:"user_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	if err := t.feishu.SendTextMessage(ctx, params.UserID, params.Text); err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return fmt.Sprintf("消息已发送给用户 %s", params.UserID), nil
}

// --- send_reminder ---

type SendReminderTool struct {
	feishu *feishu.Client
	schema json.RawMessage
}

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

func (t *SendReminderTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		UserID  string `json:"user_id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	if err := t.feishu.SendRichMessage(ctx, params.UserID, params.Title, params.Content); err != nil {
		return "", fmt.Errorf("send reminder: %w", err)
	}
	return fmt.Sprintf("提醒已发送给用户 %s", params.UserID), nil
}

// --- send_team_notification ---

type SendTeamNotificationTool struct {
	feishu        *feishu.Client
	userIDs       []string
	departmentIDs []string
	schema        json.RawMessage
}

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

func (t *SendTeamNotificationTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Text  string `json:"text"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	users, err := t.feishu.CollectUsers(ctx, t.userIDs, t.departmentIDs)
	if err != nil {
		return "", fmt.Errorf("collect users: %w", err)
	}

	sent, failed := 0, 0
	for _, u := range users {
		var sendErr error
		if params.Title != "" {
			sendErr = t.feishu.SendRichMessage(ctx, u.OpenID, params.Title, params.Text)
		} else {
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
