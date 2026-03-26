package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"okr-agent/feishu"
)

// --- list_team_members 列出团队成员 ---

type ListTeamMembersTool struct {
	feishu        *feishu.Client
	userIDs       []string
	departmentIDs []string
	schema        json.RawMessage
}

func NewListTeamMembersTool(fc *feishu.Client, userIDs, departmentIDs []string) *ListTeamMembersTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})
	return &ListTeamMembersTool{feishu: fc, userIDs: userIDs, departmentIDs: departmentIDs, schema: schema}
}

func (t *ListTeamMembersTool) Name() string              { return "list_team_members" }
func (t *ListTeamMembersTool) Description() string        { return "列出所有被监控的团队成员（open_id 和姓名）" }
func (t *ListTeamMembersTool) InputSchema() json.RawMessage { return t.schema }

func (t *ListTeamMembersTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	users, err := t.feishu.CollectUsers(ctx, t.userIDs, t.departmentIDs)
	if err != nil {
		return "", fmt.Errorf("collect users: %w", err)
	}

	if len(users) == 0 {
		return "当前没有配置任何监控用户。", nil
	}

	result := fmt.Sprintf("团队成员共 %d 人：\n", len(users))
	for i, u := range users {
		name := u.Name
		if name == "" {
			name = "(未知)"
		}
		result += fmt.Sprintf("%d. %s (open_id: %s)\n", i+1, name, u.OpenID)
	}
	return result, nil
}
