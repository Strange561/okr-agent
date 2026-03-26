package feishu

import (
	"context"
	"fmt"
	"log"

	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
)

// UserInfo 保存用户的 open_id 和姓名。
type UserInfo struct {
	OpenID string
	Name   string
}

// GetDepartmentUsers 从部门获取用户（open_id + 姓名）。
func (c *Client) GetDepartmentUsers(ctx context.Context, departmentID string) ([]UserInfo, error) {
	var users []UserInfo
	pageToken := ""

	for {
		req := larkcontact.NewFindByDepartmentUserReqBuilder().
			DepartmentId(departmentID).
			DepartmentIdType("open_department_id").
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
			if user.OpenId != nil {
				name := ""
				if user.Name != nil {
					name = *user.Name
				}
				users = append(users, UserInfo{OpenID: *user.OpenId, Name: name})
			}
		}

		if resp.Data.HasMore == nil || !*resp.Data.HasMore {
			break
		}
		if resp.Data.PageToken != nil {
			pageToken = *resp.Data.PageToken
		}
	}

	return users, nil
}

// GetSubDepartmentIDs 递归获取所有子部门 ID。
func (c *Client) GetSubDepartmentIDs(ctx context.Context, parentDeptID string) ([]string, error) {
	var allDepts []string
	pageToken := ""

	for {
		req := larkcontact.NewChildrenDepartmentReqBuilder().
			DepartmentId(parentDeptID).
			DepartmentIdType("open_department_id").
			PageSize(50)
		if pageToken != "" {
			req.PageToken(pageToken)
		}

		resp, err := c.LarkClient.Contact.Department.Children(ctx, req.Build())
		if err != nil {
			return nil, fmt.Errorf("get sub departments: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("get sub departments failed: code=%d msg=%s", resp.Code, resp.Msg)
		}

		for _, dept := range resp.Data.Items {
			if dept.OpenDepartmentId != nil {
				deptID := *dept.OpenDepartmentId
				allDepts = append(allDepts, deptID)
				subDepts, err := c.GetSubDepartmentIDs(ctx, deptID)
				if err != nil {
					log.Printf("Warning: failed to get sub departments of %s: %v", deptID, err)
					continue
				}
				allDepts = append(allDepts, subDepts...)
			}
		}

		if resp.Data.HasMore == nil || !*resp.Data.HasMore {
			break
		}
		if resp.Data.PageToken != nil {
			pageToken = *resp.Data.PageToken
		}
	}

	return allDepts, nil
}

// CollectUsers 将配置中的静态用户 ID 与部门中的动态用户（递归获取）合并。
// 返回包含 open_id 和姓名的 UserInfo 列表。
func (c *Client) CollectUsers(ctx context.Context, staticIDs, departmentIDs []string) ([]UserInfo, error) {
	seen := make(map[string]bool)
	var result []UserInfo

	for _, id := range staticIDs {
		if !seen[id] {
			seen[id] = true
			result = append(result, UserInfo{OpenID: id, Name: ""})
		}
	}

	for _, deptID := range departmentIDs {
		allDepts := []string{deptID}
		subDepts, err := c.GetSubDepartmentIDs(ctx, deptID)
		if err != nil {
			log.Printf("Warning: failed to get sub departments of %s: %v", deptID, err)
		} else {
			allDepts = append(allDepts, subDepts...)
		}

		for _, d := range allDepts {
			users, err := c.GetDepartmentUsers(ctx, d)
			if err != nil {
				log.Printf("Warning: failed to get users from department %s: %v", d, err)
				continue
			}
			for _, u := range users {
				if !seen[u.OpenID] {
					seen[u.OpenID] = true
					result = append(result, u)
				}
			}
		}
	}

	log.Printf("Collected %d users total", len(result))
	return result, nil
}
