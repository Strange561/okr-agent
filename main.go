package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"okr-agent/agent"
	"okr-agent/llm"
	"okr-agent/config"
	"okr-agent/feishu"
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

	if cfg.FeishuAppID == "" || cfg.FeishuAppSecret == "" {
		log.Fatal("FEISHU_APP_ID and FEISHU_APP_SECRET are required")
	}
	if cfg.AzureEndpoint == "" || cfg.AzureAPIKey == "" || cfg.AzureDeployment == "" {
		log.Fatal("AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_API_KEY, and AZURE_OPENAI_DEPLOYMENT are required")
	}

	// 初始化客户端
	feishuClient := feishu.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret)
	llmClient := llm.NewClient(cfg.AzureEndpoint, cfg.AzureAPIKey, cfg.AzureDeployment)

	log.Printf("Using Azure OpenAI deployment: %s", llmClient.Deployment())

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

	// 创建 Agent
	ag := agent.New(llmClient, registry, store)

	// 启动调度器
	sched := scheduler.New(feishuClient, ag, store, cfg)
	if err := sched.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}
	defer sched.Stop()

	// 设置机器人的 Agent 处理函数
	bot := feishu.NewBot(feishuClient)
	bot.SetHandler(func(ctx context.Context, senderID string, _ []feishu.MentionedUser, text string) string {
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

	log.Println("OKR Agent started successfully")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
}
