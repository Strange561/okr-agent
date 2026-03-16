package feishu

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// CommandHandler is a callback for bot commands.
type CommandHandler func(ctx context.Context, senderID string, mentionedUserIDs []string) string

// Bot handles Feishu bot events via WebSocket.
type Bot struct {
	client   *Client
	handlers map[string]CommandHandler
}

// NewBot creates a new Bot instance.
func NewBot(client *Client) *Bot {
	return &Bot{
		client:   client,
		handlers: make(map[string]CommandHandler),
	}
}

// RegisterCommand registers a command handler.
func (b *Bot) RegisterCommand(command string, handler CommandHandler) {
	b.handlers[command] = handler
}

// messageContent represents the JSON content of a received message.
type messageContent struct {
	Text string `json:"text"`
}

// Start begins WebSocket long-polling for bot events.
func (b *Bot) Start() error {
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			b.handleMessage(ctx, event)
			return nil
		})

	wsClient := larkws.NewClient(b.client.AppID, b.client.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	log.Println("Bot WebSocket connecting...")
	return wsClient.Start(context.Background())
}

func (b *Bot) handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	if event.Event == nil || event.Event.Message == nil {
		return
	}

	msg := event.Event.Message
	senderID := ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil && event.Event.Sender.SenderId.UserId != nil {
		senderID = *event.Event.Sender.SenderId.UserId
	}

	if senderID == "" {
		return
	}

	// Parse message content
	var content messageContent
	if msg.Content != nil {
		if err := json.Unmarshal([]byte(*msg.Content), &content); err != nil {
			log.Printf("Failed to parse message content: %v", err)
			return
		}
	}

	text := strings.TrimSpace(content.Text)
	// Remove @bot mentions from text (format: @_user_1)
	text = cleanMentions(text)
	text = strings.TrimSpace(text)

	// Extract mentioned user IDs
	var mentionedUserIDs []string
	if msg.Mentions != nil {
		for _, mention := range msg.Mentions {
			if mention.Id != nil && mention.Id.UserId != nil {
				mentionedUserIDs = append(mentionedUserIDs, *mention.Id.UserId)
			}
		}
	}

	log.Printf("Received command: '%s' from user: %s", text, senderID)

	// Match command
	var response string
	matched := false
	for cmd, handler := range b.handlers {
		if strings.HasPrefix(text, cmd) {
			response = handler(ctx, senderID, mentionedUserIDs)
			matched = true
			break
		}
	}

	if !matched {
		response = b.helpText()
	}

	if err := b.client.SendTextMessage(ctx, senderID, response); err != nil {
		log.Printf("Failed to reply to %s: %v", senderID, err)
	}
}

func (b *Bot) helpText() string {
	return `🤖 OKR Agent 可用命令：

• 检查OKR — 检查所有监控用户的 OKR 并返回摘要
• 评价 @某人 — 针对指定人进行 OKR 评价
• 帮助 — 显示本帮助信息`
}

// cleanMentions removes @_user_N patterns from text.
func cleanMentions(text string) string {
	words := strings.Fields(text)
	var cleaned []string
	for _, w := range words {
		if !strings.HasPrefix(w, "@_user_") {
			cleaned = append(cleaned, w)
		}
	}
	return strings.Join(cleaned, " ")
}
