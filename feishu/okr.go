package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// OKRData 表示用户的 OKR 数据。
type OKRData struct {
	UserID     string      `json:"user_id"`
	OKRList    []OKRItem   `json:"okr_list"`
	FetchedAt  time.Time   `json:"fetched_at"`
}

// OKRItem 表示单个 OKR（一个 Objective 及其 Key Results）。
type OKRItem struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	PeriodID      string      `json:"period_id"`
	Permission    int         `json:"permission"`
	ObjectiveList []Objective `json:"objective_list"`
}

type ProgressRate struct {
	Percent int    `json:"percent"`
	Status  string `json:"status"`
}

type Objective struct {
	ID                        string        `json:"id"`
	Content                   string        `json:"content"`
	MentionList               []Mention     `json:"mention_list,omitempty"`
	ProgressRate              *ProgressRate `json:"progress_rate,omitempty"`
	KeyResultList             []KeyResult   `json:"kr_list"`
	ProgressRecordLastUpdated string        `json:"progress_record_last_updated_time,omitempty"`
	ProgressRateLastUpdated   string        `json:"progress_rate_percent_last_updated_time,omitempty"`
	Weight                    int           `json:"weight"`
}

type KeyResult struct {
	ID                        string        `json:"id"`
	Content                   string        `json:"content"`
	Score                     int           `json:"score"`
	Weight                    int           `json:"weight"`
	ProgressRate              *ProgressRate `json:"progress_rate,omitempty"`
	ProgressRecordLastUpdated string        `json:"progress_record_last_updated_time,omitempty"`
	ProgressRateLastUpdated   string        `json:"progress_rate_percent_last_updated_time,omitempty"`
}

type Mention struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// okrAPIResponse 表示原始 API 响应。
type okrAPIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OKRList []json.RawMessage `json:"okr_list"`
	} `json:"data"`
}

// GetUserOKRs 获取指定用户的 OKR 数据。
// month: 目标月份，格式为 "2006-01"，空字符串表示当前月份。
func (c *Client) GetUserOKRs(ctx context.Context, userID string, month string) (*OKRData, error) {
	token, err := c.getTenantAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	url := fmt.Sprintf("https://open.feishu.cn/open-apis/okr/v1/users/%s/okrs?user_id_type=open_id&offset=0&limit=10", userID)

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

	log.Printf("OKR API response: %s", string(body))

	var apiResp okrAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("OKR API error: code=%d msg=%s", apiResp.Code, apiResp.Msg)
	}

	// 月份过滤：例如 "2026-03"
	targetMonth := month
	if targetMonth == "" {
		targetMonth = time.Now().Format("2006-01")
	}

	var okrItems []OKRItem
	for _, raw := range apiResp.Data.OKRList {
		var item OKRItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("unmarshal OKR item: %w", err)
		}
		// 只保留匹配目标月份的 OKR
		if item.Name != "" && !isCurrentMonthPeriod(item.Name, targetMonth) {
			continue
		}
		okrItems = append(okrItems, item)
	}

	return &OKRData{
		UserID:    userID,
		OKRList:   okrItems,
		FetchedAt: time.Now(),
	}, nil
}

// tokenResponse 表示租户访问令牌的响应。
type tokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

// getTenantAccessToken 从飞书获取租户访问令牌。
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

// isCurrentMonthPeriod 检查周期名称是否匹配当前月份。
// 支持 "3月"、"3 月"、"2026 年 3 月"、"2026-03" 等格式。
func isCurrentMonthPeriod(periodName, currentMonth string) bool {
	if periodName == "" {
		return true
	}
	// 移除所有空格以便于匹配
	name := strings.ReplaceAll(periodName, " ", "")

	parts := strings.Split(currentMonth, "-")
	if len(parts) != 2 {
		return true
	}
	year := parts[0]
	monthNum := strings.TrimLeft(parts[1], "0") // "03" -> "3"，去掉前导零

	// 匹配年+月格式，如 "2026年3月"
	if strings.Contains(name, year) && strings.Contains(name, monthNum+"月") {
		return true
	}
	// 匹配 "2026-03" 格式
	if strings.Contains(name, currentMonth) {
		return true
	}
	return false
}

// IsOutdated 检查 OKR 是否在最近 7 天内有更新。
// 如果超过 7 天未更新则返回 true。
func IsOutdated(modifiedTime int64) bool {
	if modifiedTime == 0 {
		return true
	}
	return time.Since(time.Unix(modifiedTime, 0)) > 7*24*time.Hour
}

// parseMilliTimestamp 将毫秒时间戳字符串解析为 Unix 秒数。
func parseMilliTimestamp(s string) int64 {
	if s == "" || s == "0" {
		return 0
	}
	var ms int64
	fmt.Sscanf(s, "%d", &ms)
	return ms / 1000
}

// LatestModifiedTime 返回所有 Objective 和 KR 中最近的修改时间（Unix 秒数）。
func LatestModifiedTime(okr *OKRData) int64 {
	var latest int64
	for _, item := range okr.OKRList {
		for _, obj := range item.ObjectiveList {
			for _, ts := range []string{obj.ProgressRecordLastUpdated, obj.ProgressRateLastUpdated} {
				if t := parseMilliTimestamp(ts); t > latest {
					latest = t
				}
			}
			for _, kr := range obj.KeyResultList {
				for _, ts := range []string{kr.ProgressRecordLastUpdated, kr.ProgressRateLastUpdated} {
					if t := parseMilliTimestamp(ts); t > latest {
						latest = t
					}
				}
			}
		}
	}
	return latest
}

// FormatOKRForEvaluation 将 OKR 数据转换为可读字符串，供 LLM 进行评估。
func FormatOKRForEvaluation(data *OKRData) string {
	if data == nil || len(data.OKRList) == 0 {
		return "该用户当前没有 OKR 数据。"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("获取时间: %s\n", data.FetchedAt.Format("2006-01-02 15:04:05")))

	latest := LatestModifiedTime(data)
	if latest > 0 {
		b.WriteString(fmt.Sprintf("最近一次更新: %s\n", time.Unix(latest, 0).Format("2006-01-02 15:04:05")))
	}
	if IsOutdated(latest) {
		if latest == 0 {
			b.WriteString("⚠️ 警告：该用户的 OKR 没有任何更新记录！\n")
		} else {
			days := int(time.Since(time.Unix(latest, 0)).Hours() / 24)
			b.WriteString(fmt.Sprintf("⚠️ 警告：该用户已 %d 天未更新 OKR，请及时更新！\n", days))
		}
	}
	b.WriteString("\n")

	for _, okr := range data.OKRList {
		b.WriteString(fmt.Sprintf("=== OKR: %s ===\n", okr.Name))

		for j, obj := range okr.ObjectiveList {
			b.WriteString(fmt.Sprintf("\nObjective %d: %s\n", j+1, obj.Content))
			if obj.ProgressRate != nil {
				b.WriteString(fmt.Sprintf("  整体进度: %d%%\n", obj.ProgressRate.Percent))
			}

			if t := parseMilliTimestamp(obj.ProgressRateLastUpdated); t > 0 {
				b.WriteString(fmt.Sprintf("  最近更新: %s\n", time.Unix(t, 0).Format("2006-01-02 15:04:05")))
			}

			for k, kr := range obj.KeyResultList {
				b.WriteString(fmt.Sprintf("  KR %d: %s\n", k+1, kr.Content))
				progress := 0
				if kr.ProgressRate != nil {
					progress = kr.ProgressRate.Percent
				}
				b.WriteString(fmt.Sprintf("    进度: %d%%, 评分: %d, 权重: %d%%\n",
					progress, kr.Score, kr.Weight))
				if t := parseMilliTimestamp(kr.ProgressRateLastUpdated); t > 0 {
					b.WriteString(fmt.Sprintf("    最近更新: %s\n", time.Unix(t, 0).Format("2006-01-02 15:04:05")))
				}
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}
