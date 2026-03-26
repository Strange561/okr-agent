package feishu

import (
	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// Client 封装飞书 Lark SDK 客户端。
type Client struct {
	LarkClient *lark.Client
	AppID      string
	AppSecret  string
}

// NewClient 创建一个新的飞书客户端。SDK 自动管理 tenant_access_token。
func NewClient(appID, appSecret string) *Client {
	larkClient := lark.NewClient(appID, appSecret)
	return &Client{
		LarkClient: larkClient,
		AppID:      appID,
		AppSecret:  appSecret,
	}
}
