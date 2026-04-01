package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"okr-agent/llm"
	"okr-agent/memory"
	"okr-agent/tools"
)

const (
	MaxIterations   = 10
	MaxHistoryTurns = 20
	MaxTokens       = 4096
)

// Agent 实现带有工具调用的 ReAct 循环。
type Agent struct {
	llm      *llm.Client
	registry *tools.Registry
	store    *memory.Store
}

// New 创建一个新的 Agent。
func New(llmClient *llm.Client, registry *tools.Registry, store *memory.Store) *Agent {
	return &Agent{
		llm:      llmClient,
		registry: registry,
		store:    store,
	}
}

// Run 为指定用户执行带有对话历史的 Agent 循环。
func (a *Agent) Run(ctx context.Context, userID, text string) (*RunResult, error) {
	// 加载现有对话
	conversation, err := a.store.GetConversation(ctx, userID)
	if err != nil {
		log.Printf("Warning: failed to load conversation for %s: %v", userID, err)
	}
	if conversation == nil {
		conversation = []llm.Message{}
	}

	// 追加用户消息
	conversation = append(conversation, llm.Message{Role: "user", Content: text})

	// 使用用户上下文构建系统提示词
	uc, _ := a.store.GetUserContext(ctx, userID)
	systemPrompt := BuildSystemPrompt(uc)

	// 执行 ReAct 循环
	result, err := a.reactLoop(ctx, systemPrompt, conversation)
	if err != nil {
		return nil, err
	}

	// 保存对话
	if saveErr := a.store.SaveConversation(ctx, userID, conversation); saveErr != nil {
		log.Printf("Warning: failed to save conversation for %s: %v", userID, saveErr)
	}

	_ = a.store.TouchUserInteraction(ctx, userID)

	return result, nil
}

// RunOneShot 执行不保存对话的一次性 Agent 循环。
func (a *Agent) RunOneShot(ctx context.Context, text string) (*RunResult, error) {
	messages := []llm.Message{{Role: "user", Content: text}}
	systemPrompt := BuildSystemPrompt(nil)
	return a.reactLoop(ctx, systemPrompt, messages)
}

// reactLoop 是 Run 和 RunOneShot 共用的核心 ReAct 循环。
func (a *Agent) reactLoop(ctx context.Context, systemPrompt string, messages []llm.Message) (*RunResult, error) {
	toolParams := a.registry.GetToolParams()
	totalToolCalls := 0

	for i := 0; i < MaxIterations; i++ {
		// 截断历史并在前面添加系统消息
		truncated := truncateHistory(messages, MaxHistoryTurns)
		allMessages := make([]llm.Message, 0, len(truncated)+1)
		allMessages = append(allMessages, llm.Message{Role: "system", Content: systemPrompt})
		allMessages = append(allMessages, truncated...)

		req := llm.Request{
			MaxCompletionTokens: MaxTokens,
			Messages:  allMessages,
			Tools:     toolParams,
		}

		resp, err := a.llm.CreateMessage(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("LLM API call: %w", err)
		}

		choice := resp.Choices[0]
		log.Printf("LLM response: finish_reason=%s, tool_calls=%d, usage=%+v",
			choice.FinishReason, len(choice.Message.ToolCalls), resp.Usage)

		// 将助手消息追加到对话中
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

				messages = append(messages, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    content,
				})
			}
			continue
		}

		// 未知的 finish_reason — 返回当前已有的内容
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

func extractText(msg llm.Message) string {
	if msg.Content != "" {
		return msg.Content
	}
	return "(Agent 没有生成文本回复)"
}

func truncateHistory(messages []llm.Message, maxTurns int) []llm.Message {
	maxMessages := maxTurns * 2
	if len(messages) <= maxMessages {
		return messages
	}
	return messages[len(messages)-maxMessages:]
}
