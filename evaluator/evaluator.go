package evaluator

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const systemPrompt = `你是一位专业的 OKR 教练。请根据以下 OKR 数据评价该成员的进展：
1. OKR 是否按时更新
2. Key Result 的进度是否合理
3. 目标设定是否符合 SMART 原则
4. 给出具体的改进建议
请用简洁友好的语气，控制在 300 字以内。`

// Evaluator handles OKR evaluation using Claude.
type Evaluator struct {
	client anthropic.Client
}

// New creates a new Evaluator with the given API key.
func New(apiKey string) *Evaluator {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Evaluator{client: client}
}

// Evaluate sends OKR data to Claude and returns the evaluation text.
func (e *Evaluator) Evaluate(ctx context.Context, okrText string) (string, error) {
	resp, err := e.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_6,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(fmt.Sprintf("以下是该成员的 OKR 数据，请进行评价：\n\n%s", okrText)),
			),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude evaluation: %w", err)
	}

	for _, block := range resp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in Claude response")
}
