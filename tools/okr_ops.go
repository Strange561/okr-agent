package tools

import (
	"context"
	"encoding/json"
)

// UpdateOKRProgressTool is a stub — the Feishu OKR API has no public write endpoint.
// It guides the user to update OKR progress manually in Feishu.
type UpdateOKRProgressTool struct {
	schema json.RawMessage
}

func NewUpdateOKRProgressTool() *UpdateOKRProgressTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user_id": map[string]interface{}{
				"type":        "string",
				"description": "用户的 open_id",
			},
			"objective_id": map[string]interface{}{
				"type":        "string",
				"description": "Objective ID",
			},
			"kr_id": map[string]interface{}{
				"type":        "string",
				"description": "Key Result ID",
			},
			"progress": map[string]interface{}{
				"type":        "integer",
				"description": "目标进度百分比 (0-100)",
			},
		},
		"required": []string{"user_id", "objective_id", "kr_id", "progress"},
	})
	return &UpdateOKRProgressTool{schema: schema}
}

func (t *UpdateOKRProgressTool) Name() string        { return "update_okr_progress" }
func (t *UpdateOKRProgressTool) Description() string  { return "更新 OKR 进度（注意：飞书 OKR API 暂不支持写操作，此工具会引导用户手动更新）" }
func (t *UpdateOKRProgressTool) InputSchema() json.RawMessage { return t.schema }

func (t *UpdateOKRProgressTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var params struct {
		UserID      string `json:"user_id"`
		ObjectiveID string `json:"objective_id"`
		KRID        string `json:"kr_id"`
		Progress    int    `json:"progress"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", nil
	}

	return "飞书 OKR API 目前不支持通过 API 更新进度。请引导用户在飞书 OKR 页面中手动更新进度。" +
		"\n\n操作步骤：\n1. 打开飞书 → OKR\n2. 找到对应的 KR\n3. 点击进度条更新百分比\n4. 添加进展记录", nil
}
