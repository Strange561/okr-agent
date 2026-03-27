package tools

import (
	"context"
	"encoding/json"
)

// ================================================================================
// update_okr_progress 工具 —— 更新 OKR 进度（桩实现）
//
// 这是一个特殊的"桩"工具：飞书 OKR API 目前没有公开的写入接口，
// 无法通过 API 直接更新 OKR 进度。因此该工具的实际行为是
// 返回操作指引，引导用户在飞书客户端中手动更新进度。
//
// 保留这个工具的原因：
//   1. 当用户或 Agent 尝试更新 OKR 进度时，提供清晰的操作指引
//   2. 当飞书未来开放写入 API 时，可以在此基础上快速实现真正的更新功能
//   3. 让 LLM 知道这个能力的存在和限制，避免"幻觉"出不存在的更新功能
// ================================================================================

// UpdateOKRProgressTool 是一个桩实现 -- 飞书 OKR API 没有公开的写入接口。
// 它引导用户在飞书中手动更新 OKR 进度。
//
// 虽然无法真正执行更新操作，但作为工具注册到 Agent 中有重要意义：
// LLM 会看到这个工具的描述中包含"暂不支持写操作"的说明，
// 从而在回复用户时给出正确的引导，而不是承诺无法实现的功能。
type UpdateOKRProgressTool struct {
	schema json.RawMessage // 预编译的 JSON Schema
}

// NewUpdateOKRProgressTool 创建更新 OKR 进度工具的实例。
//
// Schema 定义了四个参数（虽然当前不会真正使用，但保持完整的参数定义
// 有助于 LLM 理解这个操作本应需要什么信息）：
//   - user_id（必需）：用户的 open_id
//   - objective_id（必需）：Objective 的 ID
//   - kr_id（必需）：Key Result 的 ID
//   - progress（必需）：目标进度百分比 (0-100)
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

// Execute 执行"更新 OKR 进度"操作。
//
// 由于飞书 OKR API 不支持写入，该方法只是解析参数后返回操作指引文本。
// 返回的文本包含在飞书客户端中手动更新进度的步骤说明，
// LLM 会将这些步骤转述给用户。
//
// 注意：即使参数解析失败也不返回错误（返回空字符串和 nil），
// 因为参数的值在当前实现中并不重要。
func (t *UpdateOKRProgressTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	// 虽然当前不会真正使用这些参数，但仍然解析它们以保持接口一致性
	var params struct {
		UserID      string `json:"user_id"`
		ObjectiveID string `json:"objective_id"`
		KRID        string `json:"kr_id"`
		Progress    int    `json:"progress"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", nil
	}

	// 返回操作指引，告知用户需要在飞书客户端中手动操作
	return "飞书 OKR API 目前不支持通过 API 更新进度。请引导用户在飞书 OKR 页面中手动更新进度。" +
		"\n\n操作步骤：\n1. 打开飞书 → OKR\n2. 找到对应的 KR\n3. 点击进度条更新百分比\n4. 添加进展记录", nil
}
