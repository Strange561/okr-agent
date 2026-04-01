package feishu

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// MentionedUser 保存被提及用户的信息。
type MentionedUser struct {
	OpenID string
	Name   string
}

// MessageHandler 处理任意接收到的消息并返回响应。
type MessageHandler func(ctx context.Context, senderID string, mentionedUsers []MentionedUser, text string) string

// Bot 通过 WebSocket 处理飞书机器人事件。
type Bot struct {
	client  *Client
	handler MessageHandler

	// 去重：跟踪已处理的消息 ID，避免重复处理
	seen   map[string]time.Time
	seenMu sync.Mutex

	// 停止信号
	cancel context.CancelFunc
	done   chan struct{}
}

// NewBot 创建一个新的 Bot 实例。handler 必须在启动前设置。
func NewBot(client *Client, handler MessageHandler) *Bot {
	b := &Bot{
		client:  client,
		handler: handler,
		seen:    make(map[string]time.Time),
		done:    make(chan struct{}),
	}
	// 后台清理过期的去重记录，防止内存泄漏
	go b.cleanupLoop()
	return b
}

// messageContent 表示接收到的消息的 JSON 内容。
type messageContent struct {
	Text string `json:"text"`
}

// Start 启动 WebSocket 长轮询以接收机器人事件。
func (b *Bot) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel

	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			b.handleMessage(ctx, event)
			return nil
		})

	wsClient := larkws.NewClient(b.client.AppID, b.client.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelDebug),
	)

	log.Println("Bot WebSocket connecting...")
	err := wsClient.Start(ctx)
	close(b.done)
	return err
}

// Stop 停止 Bot WebSocket 连接。
func (b *Bot) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	<-b.done
}

// cleanupLoop 定期清理过期的事件去重记录，防止 seen map 无限增长。
func (b *Bot) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.seenMu.Lock()
			cutoff := time.Now().Add(-2 * time.Hour)
			for id, t := range b.seen {
				if t.Before(cutoff) {
					delete(b.seen, id)
				}
			}
			b.seenMu.Unlock()
		case <-b.done:
			return
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	if event.Event == nil || event.Event.Message == nil {
		return
	}

	// 通过 event_id 去重
	if event.EventV2Base != nil && event.EventV2Base.Header != nil && event.EventV2Base.Header.EventID != "" {
		eventID := event.EventV2Base.Header.EventID
		b.seenMu.Lock()
		if _, dup := b.seen[eventID]; dup {
			b.seenMu.Unlock()
			log.Printf("Skipping duplicate event: %s", eventID)
			return
		}
		b.seen[eventID] = time.Now()
		b.seenMu.Unlock()
	}

	msg := event.Event.Message

	senderID := ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil && event.Event.Sender.SenderId.OpenId != nil {
		senderID = *event.Event.Sender.SenderId.OpenId
	}

	if senderID == "" {
		return
	}

	// 解析消息内容
	var content messageContent
	if msg.Content != nil {
		if err := json.Unmarshal([]byte(*msg.Content), &content); err != nil {
			log.Printf("Failed to parse message content: %v", err)
			return
		}
	}

	text := strings.TrimSpace(content.Text)
	text = cleanMentions(text)
	text = strings.TrimSpace(text)

	// 提取被提及的用户
	var mentionedUsers []MentionedUser
	if msg.Mentions != nil {
		for _, mention := range msg.Mentions {
			if mention.Id != nil && mention.Id.OpenId != nil {
				name := ""
				if mention.Name != nil {
					name = *mention.Name
				}
				mentionedUsers = append(mentionedUsers, MentionedUser{OpenID: *mention.Id.OpenId, Name: name})
			}
		}
	}

	log.Printf("Received message: '%s' from user: %s", text, senderID)

	if b.handler == nil {
		log.Println("No handler registered")
		return
	}

	// 将提及信息添加到文本中，以便 Agent 知道谁被提及
	enrichedText := text
	if len(mentionedUsers) > 0 {
		var mentions []string
		for _, u := range mentionedUsers {
			name := u.Name
			if name == "" {
				name = "unknown"
			}
			mentions = append(mentions, name+" (open_id: "+u.OpenID+")")
		}
		enrichedText = text + "\n\n[提及的用户: " + strings.Join(mentions, ", ") + "]"
	}

	response := b.handler(ctx, senderID, mentionedUsers, enrichedText)

	if response != "" {
		if err := b.client.SendTextMessage(ctx, senderID, response); err != nil {
			log.Printf("Failed to reply to %s: %v", senderID, err)
		}
	}
}

// cleanMentions 从文本中移除 @_user_N 格式的内容。
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
