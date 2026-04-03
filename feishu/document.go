package feishu

import (
	"context"
	"fmt"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	larkwiki "github.com/larksuite/oapi-sdk-go/v3/service/wiki/v2"
)

// DocComment 表示文档的一条评论（简化后供 Agent 使用）。
type DocComment struct {
	CommentID string
	UserID    string
	Quote     string // 局部评论引用的文本片段
	IsWhole   bool   // 是否全文评论
	IsSolved  bool
	Replies   []DocCommentReply
}

// DocCommentReply 表示评论的一条回复。
type DocCommentReply struct {
	ReplyID string
	UserID  string
	Content string // 纯文本内容（从 Elements 提取）
}

// DocBlock 表示文档中的一个块（简化后供 Agent 使用）。
type DocBlock struct {
	BlockID     string
	BlockType   int    // 块类型：1=page, 2=text, 3-11=heading1-9, 12=bullet, 13=ordered, 14=code, 15=quote, ...
	TextContent string // 块的纯文本内容
}

// ListDocComments 获取文档的评论列表。
//
// 参数：
//   - fileToken: 文档的 file_token
//   - fileType: 文档类型（docx、doc、wiki、sheet 等）
//   - onlyUnsolved: 是否只返回未解决的评论
//
// 使用 Lark SDK 的 Drive.FileComment.List 方法，支持分页。
func (c *Client) ListDocComments(ctx context.Context, fileToken string, fileType string, onlyUnsolved bool) ([]DocComment, error) {
	if fileType == "" {
		fileType = "docx"
	}
	var allComments []DocComment
	pageToken := ""

	for {
		builder := larkdrive.NewListFileCommentReqBuilder().
			FileToken(fileToken).
			FileType(fileType).
			UserIdType("open_id").
			PageSize(50)

		if onlyUnsolved {
			builder.IsSolved(false)
		}
		if pageToken != "" {
			builder.PageToken(pageToken)
		}

		resp, err := c.LarkClient.Drive.FileComment.List(ctx, builder.Build())
		if err != nil {
			return nil, fmt.Errorf("list comments: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("list comments failed: code=%d msg=%s", resp.Code, resp.Msg)
		}

		for _, item := range resp.Data.Items {
			comment := DocComment{
				CommentID: derefStr(item.CommentId),
				UserID:    derefStr(item.UserId),
				Quote:     derefStr(item.Quote),
				IsWhole:   derefBool(item.IsWhole),
				IsSolved:  derefBool(item.IsSolved),
			}
			if item.ReplyList != nil {
				for _, reply := range item.ReplyList.Replies {
					comment.Replies = append(comment.Replies, DocCommentReply{
						ReplyID: derefStr(reply.ReplyId),
						UserID:  derefStr(reply.UserId),
						Content: extractReplyText(reply.Content),
					})
				}
			}
			allComments = append(allComments, comment)
		}

		if resp.Data.HasMore == nil || !*resp.Data.HasMore {
			break
		}
		pageToken = derefStr(resp.Data.PageToken)
		if pageToken == "" {
			break
		}
	}

	return allComments, nil
}

// GetDocContent 获取文档的纯文本内容。
func (c *Client) GetDocContent(ctx context.Context, documentID string) (string, error) {
	req := larkdocx.NewRawContentDocumentReqBuilder().
		DocumentId(documentID).
		Build()

	resp, err := c.LarkClient.Docx.Document.RawContent(ctx, req)
	if err != nil {
		return "", fmt.Errorf("get doc content: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("get doc content failed: code=%d msg=%s", resp.Code, resp.Msg)
	}

	content := derefStr(resp.Data.Content)
	// 截断过长内容，避免超出 LLM 上下文
	if len(content) > 8000 {
		content = content[:8000] + "\n...(内容过长，已截断)"
	}
	return content, nil
}

// ListDocBlocks 获取文档的所有块列表，包含块 ID 和文本内容。
//
// 仅返回包含文本内容的块（文本、标题、列表等），用于定位需要修改的块。
func (c *Client) ListDocBlocks(ctx context.Context, documentID string) ([]DocBlock, error) {
	var allBlocks []DocBlock
	pageToken := ""

	for {
		builder := larkdocx.NewListDocumentBlockReqBuilder().
			DocumentId(documentID).
			PageSize(500).
			DocumentRevisionId(-1)

		if pageToken != "" {
			builder.PageToken(pageToken)
		}

		resp, err := c.LarkClient.Docx.DocumentBlock.List(ctx, builder.Build())
		if err != nil {
			return nil, fmt.Errorf("list blocks: %w", err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("list blocks failed: code=%d msg=%s", resp.Code, resp.Msg)
		}

		for _, block := range resp.Data.Items {
			blockType := 0
			if block.BlockType != nil {
				blockType = *block.BlockType
			}
			text := extractBlockText(block)
			if text == "" {
				continue // 跳过无文本内容的块
			}
			allBlocks = append(allBlocks, DocBlock{
				BlockID:     derefStr(block.BlockId),
				BlockType:   blockType,
				TextContent: text,
			})
		}

		if resp.Data.HasMore == nil || !*resp.Data.HasMore {
			break
		}
		pageToken = derefStr(resp.Data.PageToken)
		if pageToken == "" {
			break
		}
	}

	return allBlocks, nil
}

// UpdateDocBlock 更新文档中指定块的文本内容。
//
// 使用 Patch API 替换块的所有文本元素，revision=-1 表示基于最新版本。
// 注意：此操作会替换块中所有文本元素，富文本格式会丢失。
func (c *Client) UpdateDocBlock(ctx context.Context, documentID, blockID, newText string) error {
	textRun := larkdocx.NewTextRunBuilder().
		Content(newText).
		Build()
	element := larkdocx.NewTextElementBuilder().
		TextRun(textRun).
		Build()
	updateReq := larkdocx.NewUpdateTextElementsRequestBuilder().
		Elements([]*larkdocx.TextElement{element}).
		Build()
	blockReq := larkdocx.NewUpdateBlockRequestBuilder().
		UpdateTextElements(updateReq).
		Build()

	req := larkdocx.NewPatchDocumentBlockReqBuilder().
		DocumentId(documentID).
		BlockId(blockID).
		DocumentRevisionId(-1).
		UpdateBlockRequest(blockReq).
		Build()

	resp, err := c.LarkClient.Docx.DocumentBlock.Patch(ctx, req)
	if err != nil {
		return fmt.Errorf("update block: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("update block failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// ReplyToComment 回复文档中的一条评论。
//
// 通过 Create FileComment 并设置 CommentId，SDK 会将其视为对已有评论的回复。
func (c *Client) ReplyToComment(ctx context.Context, fileToken, fileType, commentID, replyText string) error {
	if fileType == "" {
		fileType = "docx"
	}
	textRun := larkdrive.NewTextRunBuilder().Text(replyText).Build()
	element := larkdrive.NewReplyElementBuilder().
		Type("text_run").
		TextRun(textRun).
		Build()
	content := larkdrive.NewReplyContentBuilder().
		Elements([]*larkdrive.ReplyElement{element}).
		Build()
	reply := larkdrive.NewFileCommentReplyBuilder().
		Content(content).
		Build()
	replyList := larkdrive.NewReplyListBuilder().
		Replies([]*larkdrive.FileCommentReply{reply}).
		Build()
	comment := larkdrive.NewFileCommentBuilder().
		CommentId(commentID).
		ReplyList(replyList).
		Build()

	req := larkdrive.NewCreateFileCommentReqBuilder().
		FileToken(fileToken).
		FileType(fileType).
		UserIdType("open_id").
		FileComment(comment).
		Build()

	resp, err := c.LarkClient.Drive.FileComment.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("reply to comment: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("reply to comment failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// FormatCommentsForAgent 将评论列表格式化为 LLM 可读的文本。
func FormatCommentsForAgent(comments []DocComment) string {
	if len(comments) == 0 {
		return "该文档没有评论。"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 条评论：\n\n", len(comments)))

	for i, c := range comments {
		status := "未解决"
		if c.IsSolved {
			status = "已解决"
		}
		sb.WriteString(fmt.Sprintf("=== 评论 %d (comment_id: %s, %s) ===\n", i+1, c.CommentID, status))
		if c.Quote != "" {
			sb.WriteString(fmt.Sprintf("引用文本: 「%s」\n", c.Quote))
		} else if c.IsWhole {
			sb.WriteString("全文评论\n")
		}
		for _, r := range c.Replies {
			sb.WriteString(fmt.Sprintf("  回复 (user: %s): %s\n", r.UserID, r.Content))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// extractReplyText 从 ReplyContent 中提取纯文本。
func extractReplyText(content *larkdrive.ReplyContent) string {
	if content == nil {
		return ""
	}
	var parts []string
	for _, elem := range content.Elements {
		if elem.TextRun != nil && elem.TextRun.Text != nil {
			parts = append(parts, *elem.TextRun.Text)
		}
	}
	return strings.Join(parts, "")
}

// extractBlockText 从 Block 中提取纯文本内容。
//
// 支持的块类型：page(1), text(2), heading1-9(3-11), bullet(12), ordered(13),
// code(14), quote(15), todo(17)。
func extractBlockText(block *larkdocx.Block) string {
	var text *larkdocx.Text
	if block.BlockType == nil {
		return ""
	}
	switch *block.BlockType {
	case 1:
		text = block.Page
	case 2:
		text = block.Text
	case 3:
		text = block.Heading1
	case 4:
		text = block.Heading2
	case 5:
		text = block.Heading3
	case 6:
		text = block.Heading4
	case 7:
		text = block.Heading5
	case 8:
		text = block.Heading6
	case 9:
		text = block.Heading7
	case 10:
		text = block.Heading8
	case 11:
		text = block.Heading9
	case 12:
		text = block.Bullet
	case 13:
		text = block.Ordered
	case 14:
		text = block.Code
	case 15:
		text = block.Quote
	case 17:
		text = block.Todo
	default:
		return ""
	}
	if text == nil {
		return ""
	}
	var parts []string
	for _, elem := range text.Elements {
		if elem.TextRun != nil && elem.TextRun.Content != nil {
			parts = append(parts, *elem.TextRun.Content)
		}
	}
	return strings.Join(parts, "")
}

// blockTypeName 返回块类型的中文名称。
func blockTypeName(blockType int) string {
	switch blockType {
	case 1:
		return "文档"
	case 2:
		return "文本"
	case 3:
		return "一级标题"
	case 4:
		return "二级标题"
	case 5:
		return "三级标题"
	case 6, 7, 8, 9, 10, 11:
		return fmt.Sprintf("%d级标题", blockType-2)
	case 12:
		return "无序列表"
	case 13:
		return "有序列表"
	case 14:
		return "代码块"
	case 15:
		return "引用"
	case 17:
		return "待办"
	default:
		return "其他"
	}
}

// ResolveWikiToken 将 wiki node token 解析为底层文档的 obj_token 和 obj_type。
//
// 飞书 wiki 页面底层是 docx/doc/sheet 等文档类型，评论 API 需要使用真实的 obj_token。
func (c *Client) ResolveWikiToken(ctx context.Context, wikiToken string) (objToken string, objType string, err error) {
	req := larkwiki.NewGetNodeSpaceReqBuilder().
		Token(wikiToken).
		Build()

	resp, err := c.LarkClient.Wiki.Space.GetNode(ctx, req)
	if err != nil {
		return "", "", fmt.Errorf("get wiki node: %w", err)
	}
	if !resp.Success() {
		return "", "", fmt.Errorf("get wiki node failed: code=%d msg=%s", resp.Code, resp.Msg)
	}

	node := resp.Data.Node
	if node == nil {
		return "", "", fmt.Errorf("wiki node not found")
	}
	return derefStr(node.ObjToken), derefStr(node.ObjType), nil
}

// derefStr 安全地解引用字符串指针。
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// derefBool 安全地解引用布尔指针。
func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}
