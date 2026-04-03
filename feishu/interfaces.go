package feishu

import "context"

// OKRService 定义 OKR 数据查询能力，用于解耦和测试。
type OKRService interface {
	GetUserOKRs(ctx context.Context, userID string, month string) (*OKRData, error)
}

// MessageService 定义消息发送能力。
type MessageService interface {
	SendTextMessage(ctx context.Context, userID, text string) error
	SendRichMessage(ctx context.Context, userID, title, contentText string) error
}

// UserService 定义用户查询能力。
type UserService interface {
	CollectUsers(ctx context.Context, staticIDs, departmentIDs []string) ([]UserInfo, error)
}

// DocumentService 定义文档操作能力。
type DocumentService interface {
	ListDocComments(ctx context.Context, fileToken, fileType string, onlyUnsolved bool) ([]DocComment, error)
	GetDocContent(ctx context.Context, documentID string) (string, error)
	ListDocBlocks(ctx context.Context, documentID string) ([]DocBlock, error)
	UpdateDocBlock(ctx context.Context, documentID, blockID, newText string) error
	ReplyToComment(ctx context.Context, fileToken, fileType, commentID, replyText string) error
}
