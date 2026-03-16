package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OKRData represents a user's OKR data.
type OKRData struct {
	UserID     string      `json:"user_id"`
	OKRList    []OKRItem   `json:"okr_list"`
	FetchedAt  time.Time   `json:"fetched_at"`
}

// OKRItem represents a single OKR (one Objective with its Key Results).
type OKRItem struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Permission   int         `json:"permission"`
	Period       *OKRPeriod  `json:"period"`
	ObjectiveList []Objective `json:"objective_list"`
}

type OKRPeriod struct {
	PeriodID string `json:"period_id"`
	Name     string `json:"zh_name"`
}

type Objective struct {
	ID             string       `json:"id"`
	Content        string       `json:"content"`
	MentionList    []Mention    `json:"mention_list,omitempty"`
	ProgressRate   int          `json:"progress_rate"`
	KeyResultList  []KeyResult  `json:"kr_list"`
	ModifiedTime   int64        `json:"modified_time,omitempty"`
}

type KeyResult struct {
	ID           string  `json:"id"`
	Content      string  `json:"content"`
	Score        int     `json:"score"`
	Weight       int     `json:"weight"`
	ProgressRate int     `json:"progress_rate"`
	ModifiedTime int64   `json:"modified_time,omitempty"`
}

type Mention struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// okrAPIResponse represents the raw API response.
type okrAPIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OKRList []json.RawMessage `json:"okr_list"`
	} `json:"data"`
}

// GetUserOKRs fetches OKR data for a given user.
// The Lark SDK doesn't have a built-in OKR method, so we call the REST API directly.
func (c *Client) GetUserOKRs(ctx context.Context, userID string) (*OKRData, error) {
	token, err := c.getTenantAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/okr/v1/users/%s/okrs", userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp okrAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("OKR API error: code=%d msg=%s", apiResp.Code, apiResp.Msg)
	}

	var okrItems []OKRItem
	for _, raw := range apiResp.Data.OKRList {
		var item OKRItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("unmarshal OKR item: %w", err)
		}
		okrItems = append(okrItems, item)
	}

	return &OKRData{
		UserID:    userID,
		OKRList:   okrItems,
		FetchedAt: time.Now(),
	}, nil
}

// tokenResponse represents the tenant access token response.
type tokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

// getTenantAccessToken obtains a tenant access token from Feishu.
func (c *Client) getTenantAccessToken(ctx context.Context) (string, error) {
	payload := fmt.Sprintf(`{"app_id":"%s","app_secret":"%s"}`, c.AppID, c.AppSecret)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	if tokenResp.Code != 0 {
		return "", fmt.Errorf("token error: code=%d msg=%s", tokenResp.Code, tokenResp.Msg)
	}

	return tokenResp.TenantAccessToken, nil
}

// FormatOKRForEvaluation converts OKR data into a readable string for Claude evaluation.
func FormatOKRForEvaluation(data *OKRData) string {
	if data == nil || len(data.OKRList) == 0 {
		return "该用户当前没有 OKR 数据。"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("用户 ID: %s\n", data.UserID))
	b.WriteString(fmt.Sprintf("获取时间: %s\n\n", data.FetchedAt.Format("2006-01-02 15:04:05")))

	for i, okr := range data.OKRList {
		b.WriteString(fmt.Sprintf("=== OKR #%d: %s ===\n", i+1, okr.Title))
		if okr.Period != nil {
			b.WriteString(fmt.Sprintf("周期: %s\n", okr.Period.Name))
		}

		for j, obj := range okr.ObjectiveList {
			b.WriteString(fmt.Sprintf("\nObjective %d: %s\n", j+1, obj.Content))
			b.WriteString(fmt.Sprintf("  整体进度: %d%%\n", obj.ProgressRate))

			if obj.ModifiedTime > 0 {
				t := time.Unix(obj.ModifiedTime, 0)
				b.WriteString(fmt.Sprintf("  最近更新: %s\n", t.Format("2006-01-02 15:04:05")))
			}

			for k, kr := range obj.KeyResultList {
				b.WriteString(fmt.Sprintf("  KR %d: %s\n", k+1, kr.Content))
				b.WriteString(fmt.Sprintf("    进度: %d%%, 评分: %d, 权重: %d%%\n",
					kr.ProgressRate, kr.Score, kr.Weight))
				if kr.ModifiedTime > 0 {
					t := time.Unix(kr.ModifiedTime, 0)
					b.WriteString(fmt.Sprintf("    最近更新: %s\n", t.Format("2006-01-02 15:04:05")))
				}
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}
