package feishu

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// MentionedUser holds info about a mentioned user.
type MentionedUser struct {
	OpenID string
	Name   string
}

// MessageHandler processes any incoming message and returns a response.
type MessageHandler func(ctx context.Context, senderID string, mentionedUsers []MentionedUser, text string) string

// Bot handles Feishu bot events via WebSocket.
type Bot struct {
	client  *Client
	handler MessageHandler

	// Dedup: track processed message IDs to avoid retries
	seen   map[string]bool
	seenMu sync.Mutex
}

// NewBot creates a new Bot instance.
func NewBot(client *Client) *Bot {
	return &Bot{
		client: client,
		seen:   make(map[string]bool),
	}
}

// SetHandler sets the single message handler that routes all messages to the agent.
func (b *Bot) SetHandler(handler MessageHandler) {
	b.handler = handler
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
		larkws.WithLogLevel(larkcore.LogLevelDebug),
	)

	log.Println("Bot WebSocket connecting...")
	return wsClient.Start(context.Background())
}

func (b *Bot) handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	if event.Event == nil || event.Event.Message == nil {
		return
	}

	// Dedup by event_id
	if event.EventV2Base != nil && event.EventV2Base.Header != nil {
		eventID := event.EventV2Base.Header.EventID
		b.seenMu.Lock()
		if b.seen[eventID] {
			b.seenMu.Unlock()
			log.Printf("Skipping duplicate event: %s", eventID)
			return
		}
		b.seen[eventID] = true
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

	// Parse message content
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

	// Extract mentioned users
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

	// Enrich text with mention information so the agent knows who was mentioned
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
