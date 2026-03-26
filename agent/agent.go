package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"okr-agent/claude"
	"okr-agent/memory"
	"okr-agent/tools"
)

const (
	MaxIterations   = 10
	MaxHistoryTurns = 20
	MaxTokens       = 4096
)

// Agent implements the ReAct loop with tool calling.
type Agent struct {
	claude   *claude.Client
	registry *tools.Registry
	store    *memory.Store
}

// New creates a new Agent.
func New(claudeClient *claude.Client, registry *tools.Registry, store *memory.Store) *Agent {
	return &Agent{
		claude:   claudeClient,
		registry: registry,
		store:    store,
	}
}

// Run executes the agent loop with conversation history for the given user.
func (a *Agent) Run(ctx context.Context, userID, text string) (*RunResult, error) {
	// Load existing conversation
	conversation, err := a.store.GetConversation(ctx, userID)
	if err != nil {
		log.Printf("Warning: failed to load conversation for %s: %v", userID, err)
	}
	if conversation == nil {
		conversation = []claude.Message{}
	}

	// Append user message
	conversation = append(conversation, claude.Message{Role: "user", Content: text})

	// Build system prompt with user context
	uc, _ := a.store.GetUserContext(ctx, userID)
	systemPrompt := BuildSystemPrompt(uc)

	// Run the ReAct loop
	result, err := a.reactLoop(ctx, systemPrompt, conversation)
	if err != nil {
		return nil, err
	}

	// Save conversation
	if saveErr := a.store.SaveConversation(ctx, userID, conversation); saveErr != nil {
		log.Printf("Warning: failed to save conversation for %s: %v", userID, saveErr)
	}

	_ = a.store.TouchUserInteraction(ctx, userID)

	return result, nil
}

// RunOneShot executes the agent loop without conversation persistence.
func (a *Agent) RunOneShot(ctx context.Context, text string) (*RunResult, error) {
	messages := []claude.Message{{Role: "user", Content: text}}
	systemPrompt := BuildSystemPrompt(nil)
	return a.reactLoop(ctx, systemPrompt, messages)
}

// reactLoop is the core ReAct loop shared by Run and RunOneShot.
func (a *Agent) reactLoop(ctx context.Context, systemPrompt string, messages []claude.Message) (*RunResult, error) {
	toolParams := a.registry.GetToolParams()
	totalToolCalls := 0

	for i := 0; i < MaxIterations; i++ {
		// Truncate and prepend system message
		truncated := truncateHistory(messages, MaxHistoryTurns)
		allMessages := make([]claude.Message, 0, len(truncated)+1)
		allMessages = append(allMessages, claude.Message{Role: "system", Content: systemPrompt})
		allMessages = append(allMessages, truncated...)

		req := claude.Request{
			MaxCompletionTokens: MaxTokens,
			Messages:  allMessages,
			Tools:     toolParams,
		}

		resp, err := a.claude.CreateMessage(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("azure openai API call: %w", err)
		}

		choice := resp.Choices[0]
		log.Printf("LLM response: finish_reason=%s, tool_calls=%d, usage=%+v",
			choice.FinishReason, len(choice.Message.ToolCalls), resp.Usage)

		// Append assistant message to conversation
		messages = append(messages, choice.Message)

		if choice.FinishReason == "stop" || choice.FinishReason == "length" {
			return &RunResult{
				Response:  extractText(choice.Message),
				ToolCalls: totalToolCalls,
			}, nil
		}

		if choice.FinishReason == "tool_calls" {
			for _, tc := range choice.Message.ToolCalls {
				log.Printf("Executing tool: %s (id=%s)", tc.Function.Name, tc.ID)
				totalToolCalls++

				output, execErr := a.registry.Execute(ctx, tc.Function.Name, json.RawMessage(tc.Function.Arguments))

				content := output
				if execErr != nil {
					log.Printf("Tool %s error: %v", tc.Function.Name, execErr)
					content = fmt.Sprintf("Error: %s", execErr.Error())
				}

				messages = append(messages, claude.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    content,
				})
			}
			continue
		}

		// Unknown finish_reason — return what we have
		log.Printf("Unknown finish_reason: %s", choice.FinishReason)
		return &RunResult{
			Response:  extractText(choice.Message),
			ToolCalls: totalToolCalls,
		}, nil
	}

	return &RunResult{
		Response:  "抱歉，我处理这个请求花了太长时间。请尝试简化你的问题。",
		ToolCalls: totalToolCalls,
	}, nil
}

func extractText(msg claude.Message) string {
	if msg.Content != "" {
		return msg.Content
	}
	return "(Agent 没有生成文本回复)"
}

func truncateHistory(messages []claude.Message, maxTurns int) []claude.Message {
	maxMessages := maxTurns * 2
	if len(messages) <= maxMessages {
		return messages
	}
	return messages[len(messages)-maxMessages:]
}
