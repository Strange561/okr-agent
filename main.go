package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"okr-agent/agent"
	"okr-agent/config"
	"okr-agent/feishu"
	"okr-agent/llm"
	"okr-agent/memory"
	"okr-agent/scheduler"
	"okr-agent/tools"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// 初始化客户端
	feishuClient := feishu.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret)
	llmClient := llm.NewClient(cfg.LLMEndpoint, cfg.LLMAPIKey, cfg.LLMModel)

	log.Printf("Using LLM model: %s", llmClient.Model())

	// 初始化内存存储
	store, err := memory.NewStore(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer store.Close()

	log.Printf("Database initialized at: %s", cfg.SQLitePath)

	// 注册工具
	registry := tools.NewRegistry()
	registry.Register(tools.NewGetUserOKRsTool(feishuClient, store))
	registry.Register(tools.NewGetOKRHistoryTool(store))
	registry.Register(tools.NewCompareOKRPeriodsTool(feishuClient))
	registry.Register(tools.NewSendMessageTool(feishuClient))
	registry.Register(tools.NewSendReminderTool(feishuClient))
	registry.Register(tools.NewSendTeamNotificationTool(feishuClient, cfg.OKRUserIDs, cfg.DepartmentIDs))
	registry.Register(tools.NewListTeamMembersTool(feishuClient, cfg.OKRUserIDs, cfg.DepartmentIDs))
	registry.Register(tools.NewUpdateOKRProgressTool())
	registry.Register(tools.NewListDocCommentsTool(feishuClient))
	registry.Register(tools.NewGetDocContentTool(feishuClient))
	registry.Register(tools.NewListDocBlocksTool(feishuClient))
	registry.Register(tools.NewUpdateDocBlockTool(feishuClient))
	registry.Register(tools.NewReplyDocCommentTool(feishuClient))

	// 创建 Agent
	ag := agent.New(llmClient, registry, store)

	// 启动调度器
	sched := scheduler.New(feishuClient, ag, store, cfg)
	if err := sched.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}
	defer sched.Stop()

	// 设置机器人的 Agent 处理函数
	bot := feishu.NewBot(feishuClient, func(ctx context.Context, senderID string, _ []feishu.MentionedUser, text string) string {
		result, err := ag.Run(ctx, senderID, text)
		if err != nil {
			log.Printf("Agent error for user %s: %v", senderID, err)
			return "抱歉，处理你的请求时出错了，请稍后再试。"
		}
		return result.Response
	})

	go func() {
		if err := bot.Start(); err != nil {
			log.Printf("Bot error: %v", err)
		}
	}()

	// 启动健康检查 HTTP 端点
	if cfg.HealthPort != "" {
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			})
			log.Printf("Health check listening on :%s", cfg.HealthPort)
			if err := http.ListenAndServe(":"+cfg.HealthPort, mux); err != nil {
				log.Printf("Health check server error: %v", err)
			}
		}()
	}

	log.Println("OKR Agent started successfully")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	bot.Stop()
}
