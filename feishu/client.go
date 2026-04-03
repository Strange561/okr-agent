package feishu

import (
	"net/http"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// Client 封装飞书 Lark SDK 客户端。
type Client struct {
	LarkClient *lark.Client
	HTTPClient *http.Client // 带超时的 HTTP 客户端，供 OKR API 等原生 HTTP 调用使用
	AppID      string
	AppSecret  string

	// tenant_access_token 缓存，避免每次 API 调用都重新获取
	tokenCache  string
	tokenExpiry time.Time
	tokenMu     sync.Mutex
}

// NewClient 创建一个新的飞书客户端。SDK 自动管理 tenant_access_token。
func NewClient(appID, appSecret string) *Client {
	larkClient := lark.NewClient(appID, appSecret)
	return &Client{
		LarkClient: larkClient,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		AppID:      appID,
		AppSecret:  appSecret,
	}
}
