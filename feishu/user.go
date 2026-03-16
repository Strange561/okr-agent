package feishu

import (
	"context"
	"fmt"

	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
)

// GetDepartmentUserIDs fetches user IDs from a department.
func (c *Client) GetDepartmentUserIDs(ctx context.Context, departmentID string) ([]string, error) {
	var userIDs []string
	pageToken := ""

	for {
		req := larkcontact.NewFindByDepartmentUserReqBuilder().
			DepartmentId(departmentID).
			PageSize(50)
		if pageToken != "" {
			req.PageToken(pageToken)
		}

		resp, err := c.LarkClient.Contact.User.FindByDepartment(ctx, req.Build())
		if err != nil {
			return nil, fmt.Errorf("get department members: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("get department members failed: code=%d msg=%s", resp.Code, resp.Msg)
		}

		for _, user := range resp.Data.Items {
			if user.UserId != nil {
				userIDs = append(userIDs, *user.UserId)
			}
		}

		if resp.Data.HasMore == nil || !*resp.Data.HasMore {
			break
		}
		if resp.Data.PageToken != nil {
			pageToken = *resp.Data.PageToken
		}
	}

	return userIDs, nil
}

// CollectUserIDs merges static user IDs from config with dynamic ones from departments.
func (c *Client) CollectUserIDs(ctx context.Context, staticIDs, departmentIDs []string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	for _, id := range staticIDs {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}

	for _, deptID := range departmentIDs {
		ids, err := c.GetDepartmentUserIDs(ctx, deptID)
		if err != nil {
			return nil, fmt.Errorf("department %s: %w", deptID, err)
		}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}

	return result, nil
}
