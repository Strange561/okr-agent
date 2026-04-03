package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"okr-agent/feishu"
)

// ================================================================================
// list_doc_comments 工具 —— 获取文档评论列表
//
// 获取飞书文档的评论，默认只返回未解决的评论。
// Agent 通过此工具了解文档中有哪些待处理的评论。
// ================================================================================

// ListDocCommentsTool 实现获取文档评论列表的工具。
type ListDocCommentsTool struct {
	feishu *feishu.Client
	schema json.RawMessage
}

// NewListDocCommentsTool 创建获取文档评论工具的实例。
func NewListDocCommentsTool(fc *feishu.Client) *ListDocCommentsTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_token": map[string]interface{}{
				"type":        "string",
				"description": "文档的 file_token（可从文档链接中提取）",
			},
			"file_type": map[string]interface{}{
				"type":        "string",
				"description": "文档类型：docx、doc、wiki、sheet 等，根据链接中的路径判断，默认 docx",
			},
			"only_unsolved": map[string]interface{}{
				"type":        "boolean",
				"description": "是否只返回未解决的评论，默认 true",
			},
		},
		"required": []string{"file_token"},
	})
	return &ListDocCommentsTool{feishu: fc, schema: schema}
}

func (t *ListDocCommentsTool) Name() string                { return "list_doc_comments" }
func (t *ListDocCommentsTool) Description() string          { return "获取飞书文档的评论列表，默认只返回未解决的评论" }
func (t *ListDocCommentsTool) InputSchema() json.RawMessage { return t.schema }

func (t *ListDocCommentsTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		FileToken    string `json:"file_token"`
		FileType     string `json:"file_type"`
		OnlyUnsolved *bool  `json:"only_unsolved"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	onlyUnsolved := true
	if params.OnlyUnsolved != nil {
		onlyUnsolved = *params.OnlyUnsolved
	}

	comments, err := t.feishu.ListDocComments(ctx, params.FileToken, params.FileType, onlyUnsolved)
	if err != nil {
		return "", fmt.Errorf("list doc comments: %w", err)
	}

	return feishu.FormatCommentsForAgent(comments), nil
}

// ================================================================================
// get_doc_content 工具 —— 获取文档纯文本内容
//
// 获取飞书文档的纯文本内容，用于理解文档上下文。
// Agent 通过此工具了解文档的整体内容。
// ================================================================================

// GetDocContentTool 实现获取文档纯文本内容的工具。
type GetDocContentTool struct {
	feishu *feishu.Client
	schema json.RawMessage
}

// NewGetDocContentTool 创建获取文档内容工具的实例。
func NewGetDocContentTool(fc *feishu.Client) *GetDocContentTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"document_id": map[string]interface{}{
				"type":        "string",
				"description": "文档的 document_id（与 file_token 相同）",
			},
		},
		"required": []string{"document_id"},
	})
	return &GetDocContentTool{feishu: fc, schema: schema}
}

func (t *GetDocContentTool) Name() string                { return "get_doc_content" }
func (t *GetDocContentTool) Description() string          { return "获取飞书文档的纯文本内容，用于理解文档上下文" }
func (t *GetDocContentTool) InputSchema() json.RawMessage { return t.schema }

func (t *GetDocContentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		DocumentID string `json:"document_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	content, err := t.feishu.GetDocContent(ctx, params.DocumentID)
	if err != nil {
		return "", fmt.Errorf("get doc content: %w", err)
	}

	return content, nil
}

// ================================================================================
// list_doc_blocks 工具 —— 获取文档块列表
//
// 获取飞书文档的所有文本块，包含 block_id 和文本内容。
// Agent 通过此工具找到评论引用文本对应的 block_id，从而定位修改位置。
// ================================================================================

// ListDocBlocksTool 实现获取文档块列表的工具。
type ListDocBlocksTool struct {
	feishu *feishu.Client
	schema json.RawMessage
}

// NewListDocBlocksTool 创建获取文档块列表工具的实例。
func NewListDocBlocksTool(fc *feishu.Client) *ListDocBlocksTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"document_id": map[string]interface{}{
				"type":        "string",
				"description": "文档的 document_id（与 file_token 相同）",
			},
		},
		"required": []string{"document_id"},
	})
	return &ListDocBlocksTool{feishu: fc, schema: schema}
}

func (t *ListDocBlocksTool) Name() string                { return "list_doc_blocks" }
func (t *ListDocBlocksTool) Description() string          { return "获取飞书文档的所有块列表（含 block_id 和文本内容），用于定位需要修改的块" }
func (t *ListDocBlocksTool) InputSchema() json.RawMessage { return t.schema }

func (t *ListDocBlocksTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		DocumentID string `json:"document_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	blocks, err := t.feishu.ListDocBlocks(ctx, params.DocumentID)
	if err != nil {
		return "", fmt.Errorf("list doc blocks: %w", err)
	}

	if len(blocks) == 0 {
		return "文档没有文本块。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个文本块：\n\n", len(blocks)))
	for _, b := range blocks {
		sb.WriteString(fmt.Sprintf("Block %s (type=%d): %s\n", b.BlockID, b.BlockType, b.TextContent))
	}
	return sb.String(), nil
}

// ================================================================================
// update_doc_block 工具 —— 更新文档块文本
//
// 更新飞书文档中指定块的文本内容。
// Agent 通过此工具根据评论修改文档内容。
// ================================================================================

// UpdateDocBlockTool 实现更新文档块文本的工具。
type UpdateDocBlockTool struct {
	feishu *feishu.Client
	schema json.RawMessage
}

// NewUpdateDocBlockTool 创建更新文档块工具的实例。
func NewUpdateDocBlockTool(fc *feishu.Client) *UpdateDocBlockTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"document_id": map[string]interface{}{
				"type":        "string",
				"description": "文档的 document_id",
			},
			"block_id": map[string]interface{}{
				"type":        "string",
				"description": "要更新的块的 block_id",
			},
			"new_text": map[string]interface{}{
				"type":        "string",
				"description": "替换后的新文本内容",
			},
		},
		"required": []string{"document_id", "block_id", "new_text"},
	})
	return &UpdateDocBlockTool{feishu: fc, schema: schema}
}

func (t *UpdateDocBlockTool) Name() string                { return "update_doc_block" }
func (t *UpdateDocBlockTool) Description() string          { return "更新飞书文档中指定块的文本内容" }
func (t *UpdateDocBlockTool) InputSchema() json.RawMessage { return t.schema }

func (t *UpdateDocBlockTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		DocumentID string `json:"document_id"`
		BlockID    string `json:"block_id"`
		NewText    string `json:"new_text"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	if err := t.feishu.UpdateDocBlock(ctx, params.DocumentID, params.BlockID, params.NewText); err != nil {
		return "", fmt.Errorf("update doc block: %w", err)
	}

	return fmt.Sprintf("已更新块 %s 的内容", params.BlockID), nil
}

// ================================================================================
// reply_doc_comment 工具 —— 回复文档评论
//
// 回复飞书文档中的评论，用于说明已完成的修改。
// Agent 修改文档后通过此工具告知评论者修改内容。
// ================================================================================

// ReplyDocCommentTool 实现回复文档评论的工具。
type ReplyDocCommentTool struct {
	feishu *feishu.Client
	schema json.RawMessage
}

// NewReplyDocCommentTool 创建回复文档评论工具的实例。
func NewReplyDocCommentTool(fc *feishu.Client) *ReplyDocCommentTool {
	schema, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_token": map[string]interface{}{
				"type":        "string",
				"description": "文档的 file_token",
			},
			"file_type": map[string]interface{}{
				"type":        "string",
				"description": "文档类型：docx、doc、wiki、sheet 等，默认 docx",
			},
			"comment_id": map[string]interface{}{
				"type":        "string",
				"description": "要回复的评论 ID",
			},
			"reply_text": map[string]interface{}{
				"type":        "string",
				"description": "回复内容，说明做了什么修改",
			},
		},
		"required": []string{"file_token", "comment_id", "reply_text"},
	})
	return &ReplyDocCommentTool{feishu: fc, schema: schema}
}

func (t *ReplyDocCommentTool) Name() string                { return "reply_doc_comment" }
func (t *ReplyDocCommentTool) Description() string          { return "回复飞书文档中的评论，用于说明已完成的修改" }
func (t *ReplyDocCommentTool) InputSchema() json.RawMessage { return t.schema }

func (t *ReplyDocCommentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		FileToken string `json:"file_token"`
		FileType  string `json:"file_type"`
		CommentID string `json:"comment_id"`
		ReplyText string `json:"reply_text"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	if err := t.feishu.ReplyToComment(ctx, params.FileToken, params.FileType, params.CommentID, params.ReplyText); err != nil {
		return "", fmt.Errorf("reply to comment: %w", err)
	}

	return fmt.Sprintf("已回复评论 %s", params.CommentID), nil
}
