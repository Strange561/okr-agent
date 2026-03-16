package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"okr-agent/config"
	"okr-agent/evaluator"
	"okr-agent/feishu"
	"okr-agent/scheduler"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.FeishuAppID == "" || cfg.FeishuAppSecret == "" {
		log.Fatal("FEISHU_APP_ID and FEISHU_APP_SECRET are required")
	}
	if cfg.AnthropicAPIKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	// Initialize clients
	feishuClient := feishu.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret)
	eval := evaluator.New(cfg.AnthropicAPIKey)
	sched := scheduler.New(feishuClient, eval, cfg)

	// Start cron scheduler
	if err := sched.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}
	defer sched.Stop()

	// Setup bot commands
	bot := feishu.NewBot(feishuClient)

	bot.RegisterCommand("检查OKR", func(ctx context.Context, senderID string, _ []string) string {
		userIDs, err := feishuClient.CollectUserIDs(ctx, cfg.OKRUserIDs, cfg.DepartmentIDs)
		if err != nil {
			return fmt.Sprintf("获取用户列表失败: %v", err)
		}

		if len(userIDs) == 0 {
			return "未配置任何监控用户，请检查 OKR_USER_IDS 或 FEISHU_DEPARTMENT_IDS 配置。"
		}

		var results []string
		for _, uid := range userIDs {
			evaluation, err := sched.CheckSingleUser(ctx, uid)
			if err != nil {
				results = append(results, fmt.Sprintf("用户 %s: 检查失败 - %v", uid, err))
				continue
			}
			results = append(results, fmt.Sprintf("用户 %s:\n%s", uid, evaluation))
		}

		return strings.Join(results, "\n\n---\n\n")
	})

	bot.RegisterCommand("评价", func(ctx context.Context, senderID string, mentionedUserIDs []string) string {
		if len(mentionedUserIDs) == 0 {
			return "请在命令后 @mention 需要评价的用户，例如：评价 @张三"
		}

		var results []string
		for _, uid := range mentionedUserIDs {
			evaluation, err := sched.CheckSingleUser(ctx, uid)
			if err != nil {
				results = append(results, fmt.Sprintf("用户 %s: 评价失败 - %v", uid, err))
				continue
			}
			results = append(results, fmt.Sprintf("用户 %s:\n%s", uid, evaluation))
		}

		return strings.Join(results, "\n\n---\n\n")
	})

	// "帮助" and any unrecognized command will trigger the default help text in bot.go

	// Start bot in a goroutine
	go func() {
		if err := bot.Start(); err != nil {
			log.Printf("Bot error: %v", err)
		}
	}()

	log.Println("OKR Agent started successfully")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
}
