package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// SendTextMessage 通过私聊向用户发送文本消息。
func (c *Client) SendTextMessage(ctx context.Context, userID, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("open_id").
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

// SendRichMessage 向用户发送富文本（post）消息。
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
		ReceiveIdType("open_id").
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
