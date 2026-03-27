package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"okr-agent/feishu"
	"okr-agent/memory"
)

// ================================================================================
// get_user_okrs 工具 —— 获取用户当前的 OKR 数据
//
// 这是 Agent 最常用的工具之一。当用户请求查看或评估 OKR 时，
// Agent 会首先调用此工具获取飞书 OKR 系统中的最新数据。
// 获取后的数据会被自动保存为快照，支持后续的历史趋势分析。
// ================================================================================

// GetUserOKRsTool 实现获取用户 OKR 数据的工具。
//
// 该工具通过飞书 OKR API 获取指定用户的 OKR 数据，
// 并将格式化后的结果返回给 LLM 进行分析和评估。
// 同时自动保存 OKR 快照到数据库，用于趋势分析。
type GetUserOKRsTool struct {
	feishu *feishu.Client  // 飞书客户端，用于调用 OKR API
	store  *memory.Store   // 存储实例，用于保存 OKR 快照
	schema json.RawMessage // 预编译的 JSON Schema，描述工具的输入参数
}

// NewGetUserOKRsTool 创建获取用户 OKR 工具的实例。
//
// 在构造函数中预编译 JSON Schema，避免每次调用 InputSchema() 时重复序列化。
// Schema 定义了两个参数：
//   - user_id（必需）：用户的 open_id，用于标识要查询的用户
//   - month（可选）：目标月份，默认当前月份
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

// Execute 执行获取用户 OKR 数据的操作。
//
// 工作流程：
//  1. 解析 LLM 传入的参数（user_id 和可选的 month）
//  2. 调用飞书 OKR API 获取原始数据
//  3. 将原始数据格式化为人类和 LLM 可读的文本
//  4. 自动保存快照到数据库（用于趋势分析）
//  5. 返回格式化后的文本给 LLM
func (t *GetUserOKRsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	// 解析 LLM 传入的 JSON 参数
	var params struct {
		UserID string `json:"user_id"`
		Month  string `json:"month"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	// 调用飞书 API 获取 OKR 数据
	okrData, err := t.feishu.GetUserOKRs(ctx, params.UserID, params.Month)
	if err != nil {
		return "", fmt.Errorf("get user OKRs: %w", err)
	}

	// 将原始 OKR 数据格式化为可读文本，包含进度、更新时间、警告等信息
	formatted := feishu.FormatOKRForEvaluation(okrData)

	// 自动保存快照，用于后续的趋势分析和历史对比
	// 即使保存失败也不影响主流程（静默忽略错误）
	month := params.Month
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if t.store != nil {
		_ = t.store.SaveOKRSnapshot(ctx, params.UserID, month, formatted)
	}

	return formatted, nil
}

// ================================================================================
// get_okr_history 工具 —— 获取 OKR 历史快照
//
// 用于查看用户 OKR 的历史变化。Agent 使用此工具进行趋势分析，
// 例如对比上周和本周的进度变化，判断用户是否在稳步推进 OKR。
// ================================================================================

// GetOKRHistoryTool 实现获取 OKR 历史快照的工具。
//
// 该工具从数据库中读取之前保存的 OKR 快照，返回给 LLM 进行分析。
// 快照是由 GetUserOKRsTool 在每次获取 OKR 时自动保存的。
type GetOKRHistoryTool struct {
	store  *memory.Store   // 存储实例，用于查询历史快照
	schema json.RawMessage // 预编译的 JSON Schema
}

// NewGetOKRHistoryTool 创建获取 OKR 历史快照工具的实例。
//
// Schema 定义了两个参数：
//   - user_id（必需）：用户的 open_id
//   - limit（可选）：返回的快照数量上限，默认 10
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

// Execute 执行查询 OKR 历史快照的操作。
//
// 从数据库中获取指定用户的历史快照，格式化后返回给 LLM。
// 如果没有找到历史快照，会返回友好的提示信息。
func (t *GetOKRHistoryTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	// 解析 LLM 传入的参数
	var params struct {
		UserID string `json:"user_id"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	// 从数据库查询历史快照
	snapshots, err := t.store.GetOKRSnapshots(ctx, params.UserID, params.Limit)
	if err != nil {
		return "", fmt.Errorf("get snapshots: %w", err)
	}

	// 如果没有历史数据，返回提示信息
	if len(snapshots) == 0 {
		return "没有找到该用户的 OKR 历史快照。", nil
	}

	// 格式化所有快照为可读文本，每条快照包含月份和时间信息
	var result string
	for i, snap := range snapshots {
		result += fmt.Sprintf("=== 快照 %d (月份: %s, 时间: %s) ===\n%s\n\n",
			i+1, snap.Month, snap.CreatedAt.Format("2006-01-02 15:04"), snap.OKRData)
	}
	return result, nil
}

// ================================================================================
// compare_okr_periods 工具 —— 对比不同时间段的 OKR
//
// 允许 Agent 实时获取并对比同一用户在两个不同月份的 OKR 数据。
// 与 get_okr_history（查看保存的快照）不同，此工具会实时从飞书 API 获取数据。
// ================================================================================

// CompareOKRPeriodsTool 实现对比不同时间段 OKR 的工具。
//
// 通过分别获取两个月份的 OKR 数据并拼接展示，
// 让 LLM 可以直观地对比进度变化。
type CompareOKRPeriodsTool struct {
	feishu *feishu.Client  // 飞书客户端，用于获取 OKR 数据
	schema json.RawMessage // 预编译的 JSON Schema
}

// NewCompareOKRPeriodsTool 创建对比 OKR 时间段工具的实例。
//
// Schema 定义了三个必需参数：
//   - user_id：用户的 open_id
//   - month1：第一个月份，格式 YYYY-MM
//   - month2：第二个月份，格式 YYYY-MM
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

// Execute 执行 OKR 时间段对比操作。
//
// 分别调用飞书 API 获取两个月份的 OKR 数据，
// 格式化后拼接在一起返回给 LLM，便于进行对比分析。
func (t *CompareOKRPeriodsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	// 解析参数
	var params struct {
		UserID string `json:"user_id"`
		Month1 string `json:"month1"`
		Month2 string `json:"month2"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	// 分别获取两个月份的 OKR 数据
	okr1, err := t.feishu.GetUserOKRs(ctx, params.UserID, params.Month1)
	if err != nil {
		return "", fmt.Errorf("get OKRs for %s: %w", params.Month1, err)
	}

	okr2, err := t.feishu.GetUserOKRs(ctx, params.UserID, params.Month2)
	if err != nil {
		return "", fmt.Errorf("get OKRs for %s: %w", params.Month2, err)
	}

	// 将两个月份的数据格式化并拼接，便于 LLM 进行对比分析
	result := fmt.Sprintf("=== %s 的 OKR ===\n%s\n\n=== %s 的 OKR ===\n%s",
		params.Month1, feishu.FormatOKRForEvaluation(okr1),
		params.Month2, feishu.FormatOKRForEvaluation(okr2))

	return result, nil
}
