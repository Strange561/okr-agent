package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"okr-agent/feishu"
	"okr-agent/memory"
)

// --- get_user_okrs 获取用户 OKR ---

type GetUserOKRsTool struct {
	feishu *feishu.Client
	store  *memory.Store
	schema json.RawMessage
}

func NewGetUserOKRsTool(fc *feishu.Client, store *memory.Store) *GetUserOKRsTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user_id": map[string]interface{}{
				"type":        "string",
				"description": "用户的 open_id",
			},
			"month": map[string]interface{}{
				"type":        "string",
				"description": "目标月份，格式 YYYY-MM，留空表示当前月",
			},
		},
		"required": []string{"user_id"},
	})
	return &GetUserOKRsTool{feishu: fc, store: store, schema: schema}
}

func (t *GetUserOKRsTool) Name() string        { return "get_user_okrs" }
func (t *GetUserOKRsTool) Description() string  { return "获取指定用户的 OKR 数据，包括 Objective、KR、进度和更新时间" }
func (t *GetUserOKRsTool) InputSchema() json.RawMessage { return t.schema }

func (t *GetUserOKRsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		UserID string `json:"user_id"`
		Month  string `json:"month"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	okrData, err := t.feishu.GetUserOKRs(ctx, params.UserID, params.Month)
	if err != nil {
		return "", fmt.Errorf("get user OKRs: %w", err)
	}

	formatted := feishu.FormatOKRForEvaluation(okrData)

	// 自动保存快照，用于趋势分析
	month := params.Month
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if t.store != nil {
		_ = t.store.SaveOKRSnapshot(ctx, params.UserID, month, formatted)
	}

	return formatted, nil
}

// --- get_okr_history 获取 OKR 历史 ---

type GetOKRHistoryTool struct {
	store  *memory.Store
	schema json.RawMessage
}

func NewGetOKRHistoryTool(store *memory.Store) *GetOKRHistoryTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user_id": map[string]interface{}{
				"type":        "string",
				"description": "用户的 open_id",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "返回的快照数量上限，默认 10",
			},
		},
		"required": []string{"user_id"},
	})
	return &GetOKRHistoryTool{store: store, schema: schema}
}

func (t *GetOKRHistoryTool) Name() string        { return "get_okr_history" }
func (t *GetOKRHistoryTool) Description() string  { return "查询用户的 OKR 历史快照，用于趋势分析和进度对比" }
func (t *GetOKRHistoryTool) InputSchema() json.RawMessage { return t.schema }

func (t *GetOKRHistoryTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		UserID string `json:"user_id"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	snapshots, err := t.store.GetOKRSnapshots(ctx, params.UserID, params.Limit)
	if err != nil {
		return "", fmt.Errorf("get snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return "没有找到该用户的 OKR 历史快照。", nil
	}

	var result string
	for i, snap := range snapshots {
		result += fmt.Sprintf("=== 快照 %d (月份: %s, 时间: %s) ===\n%s\n\n",
			i+1, snap.Month, snap.CreatedAt.Format("2006-01-02 15:04"), snap.OKRData)
	}
	return result, nil
}

// --- compare_okr_periods 对比 OKR 时间段 ---

type CompareOKRPeriodsTool struct {
	feishu *feishu.Client
	schema json.RawMessage
}

func NewCompareOKRPeriodsTool(fc *feishu.Client) *CompareOKRPeriodsTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user_id": map[string]interface{}{
				"type":        "string",
				"description": "用户的 open_id",
			},
			"month1": map[string]interface{}{
				"type":        "string",
				"description": "第一个月份，格式 YYYY-MM",
			},
			"month2": map[string]interface{}{
				"type":        "string",
				"description": "第二个月份，格式 YYYY-MM",
			},
		},
		"required": []string{"user_id", "month1", "month2"},
	})
	return &CompareOKRPeriodsTool{feishu: fc, schema: schema}
}

func (t *CompareOKRPeriodsTool) Name() string        { return "compare_okr_periods" }
func (t *CompareOKRPeriodsTool) Description() string  { return "对比同一用户在两个不同月份的 OKR 数据" }
func (t *CompareOKRPeriodsTool) InputSchema() json.RawMessage { return t.schema }

func (t *CompareOKRPeriodsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		UserID string `json:"user_id"`
		Month1 string `json:"month1"`
		Month2 string `json:"month2"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	okr1, err := t.feishu.GetUserOKRs(ctx, params.UserID, params.Month1)
	if err != nil {
		return "", fmt.Errorf("get OKRs for %s: %w", params.Month1, err)
	}

	okr2, err := t.feishu.GetUserOKRs(ctx, params.UserID, params.Month2)
	if err != nil {
		return "", fmt.Errorf("get OKRs for %s: %w", params.Month2, err)
	}

	result := fmt.Sprintf("=== %s 的 OKR ===\n%s\n\n=== %s 的 OKR ===\n%s",
		params.Month1, feishu.FormatOKRForEvaluation(okr1),
		params.Month2, feishu.FormatOKRForEvaluation(okr2))

	return result, nil
}
