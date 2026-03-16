package feishu

import (
	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// Client wraps the Lark SDK client.
type Client struct {
	LarkClient *lark.Client
	AppID      string
	AppSecret  string
}

// NewClient creates a new Feishu client. The SDK automatically manages tenant_access_token.
func NewClient(appID, appSecret string) *Client {
	larkClient := lark.NewClient(appID, appSecret)
	return &Client{
		LarkClient: larkClient,
		AppID:      appID,
		AppSecret:  appSecret,
	}
}
