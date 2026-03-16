package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// SendTextMessage sends a text message to a user via private chat.
func (c *Client) SendTextMessage(ctx context.Context, userID, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("user_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(userID).
			MsgType("text").
			Content(string(content)).
			Build()).
		Build()

	resp, err := c.LarkClient.Im.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}

	return nil
}

// SendRichMessage sends a rich text (post) message to a user.
func (c *Client) SendRichMessage(ctx context.Context, userID, title, contentText string) error {
	post := map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"title": title,
			"content": [][]map[string]interface{}{
				{
					{"tag": "text", "text": contentText},
				},
			},
		},
	}

	content, _ := json.Marshal(post)

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("user_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(userID).
			MsgType("post").
			Content(string(content)).
			Build()).
		Build()

	resp, err := c.LarkClient.Im.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("send rich message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send rich message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}

	return nil
}
