package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"okr-agent/agent"
	"okr-agent/config"
	"okr-agent/feishu"
	"okr-agent/memory"
)

// Scheduler 管理基于 cron 的 OKR 检查，由 Agent 驱动智能决策。
type Scheduler struct {
	cron   *cron.Cron
	feishu *feishu.Client
	agent  *agent.Agent
	store  *memory.Store
	config *config.Config
}

// New 创建一个新的调度器。
func New(fc *feishu.Client, ag *agent.Agent, store *memory.Store, cfg *config.Config) *Scheduler {
	return &Scheduler{
		cron:   cron.New(),
		feishu: fc,
		agent:  ag,
		store:  store,
		config: cfg,
	}
}

// Start 启动 cron 调度器。
func (s *Scheduler) Start() error {
	// 预校验 cron 表达式
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(s.config.CronSchedule); err != nil {
		return fmt.Errorf("invalid cron schedule %q: %w", s.config.CronSchedule, err)
	}

	// 主 OKR 检查（可配置，默认：周一 9:00）
	_, err := s.cron.AddFunc(s.config.CronSchedule, func() {
		s.RunCheck()
	})
	if err != nil {
		return err
	}

	// 每日风险扫描，10:00 执行
	_, err = s.cron.AddFunc("0 10 * * *", func() {
		s.DailyRiskScan()
	})
	if err != nil {
		return err
	}

	// 周五提醒，10:00 执行
	_, err = s.cron.AddFunc("0 10 * * 5", func() {
		s.SendReminder()
	})
	if err != nil {
		return err
	}

	s.cron.Start()
	log.Printf("Scheduler started with schedule: %s", s.config.CronSchedule)
	log.Println("Daily risk scan scheduled at 10:00")
	log.Println("Friday OKR reminder scheduled at 10:00")
	return nil
}

// Stop 优雅地停止调度器。
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// RunCheck 对所有配置的用户执行 Agent 驱动的 OKR 评估。
func (s *Scheduler) RunCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	users, err := s.feishu.CollectUsers(ctx, s.config.OKRUserIDs, s.config.DepartmentIDs)
	if err != nil {
		log.Printf("Error collecting users: %v", err)
		return
	}

	log.Printf("Starting agent-driven OKR check for %d users", len(users))

	for _, u := range users {
		name := u.Name
		if name == "" {
			name = u.OpenID
		}

		prompt := "请评估用户 " + name + " (open_id: " + u.OpenID + ") 的本月 OKR。" +
			"先获取 OKR 数据，然后逐个评价每个 Objective 和 KR，最后用 send_reminder 将评估结果发送给该用户（标题用「OKR 周报评价」）。"

		result, err := s.agent.RunOneShot(ctx, prompt)
		if err != nil {
			log.Printf("Agent error for %s: %v", name, err)
			continue
		}
		log.Printf("Completed check for %s (tool_calls=%d)", name, result.ToolCalls)
	}

	log.Println("OKR check completed")
}

// DailyRiskScan 检查所有用户的 OKR 更新时间，并触发个性化提醒。
func (s *Scheduler) DailyRiskScan() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	users, err := s.feishu.CollectUsers(ctx, s.config.OKRUserIDs, s.config.DepartmentIDs)
	if err != nil {
		log.Printf("Error collecting users for risk scan: %v", err)
		return
	}

	log.Printf("Starting daily risk scan for %d users", len(users))

	for _, u := range users {
		name := u.Name
		if name == "" {
			name = u.OpenID
		}

		// 获取 OKR 数据以检查最后更新时间
		okrData, err := s.feishu.GetUserOKRs(ctx, u.OpenID, "")
		if err != nil {
			log.Printf("Risk scan: error fetching OKR for %s: %v", name, err)
			continue
		}

		lastModified := feishu.LatestModifiedTime(okrData)
		daysSinceUpdate := 0
		if lastModified > 0 {
			daysSinceUpdate = int(time.Since(time.Unix(lastModified, 0)).Hours() / 24)
		} else {
			daysSinceUpdate = 999 // 从未更新
		}

		// 使用可配置的阈值判断风险等级
		riskLevel := "normal"
		switch {
		case daysSinceUpdate >= s.config.RiskDaysCritical:
			riskLevel = "critical"
		case daysSinceUpdate >= s.config.RiskDaysHigh:
			riskLevel = "high"
		}

		// 保存状态
		state := &memory.SchedulerState{
			UserID:          u.OpenID,
			RiskLevel:       riskLevel,
			DaysSinceUpdate: daysSinceUpdate,
		}

		// 检查是否需要发送提醒
		existingState, _ := s.store.GetSchedulerState(ctx, u.OpenID)
		shouldRemind := false

		criticalCooldown := time.Duration(s.config.RiskCooldownCriticalHours) * time.Hour
		highCooldown := time.Duration(s.config.RiskCooldownHighHours) * time.Hour

		switch riskLevel {
		case "critical":
			if existingState.LastReminder == nil || time.Since(*existingState.LastReminder) > criticalCooldown {
				shouldRemind = true
			}
		case "high":
			if existingState.LastReminder == nil || time.Since(*existingState.LastReminder) > highCooldown {
				shouldRemind = true
			}
		}

		if shouldRemind {
			log.Printf("Risk scan: %s is %s (days=%d), generating reminder", name, riskLevel, daysSinceUpdate)

			prompt := fmt.Sprintf(
				"用户 %s (open_id: %s) 已经 %d 天没有更新 OKR。风险等级：%s。"+
					"请先查看该用户的 OKR 数据，然后生成一条个性化的提醒消息并发送给该用户。"+
					"提醒应当友好但有紧迫感，提及具体的 OKR 内容。",
				name, u.OpenID, daysSinceUpdate, riskLevel)

			result, err := s.agent.RunOneShot(ctx, prompt)
			if err != nil {
				log.Printf("Agent reminder error for %s: %v", name, err)
			} else {
				log.Printf("Sent personalized reminder to %s (tool_calls=%d)", name, result.ToolCalls)
				state.LastReminder = timePtr(time.Now())
				if err := s.store.UpdateReminderTime(ctx, u.OpenID); err != nil {
					log.Printf("Error updating reminder time for %s: %v", name, err)
				}
			}
		}

		if err := s.store.SaveSchedulerState(ctx, state); err != nil {
			log.Printf("Error saving scheduler state for %s: %v", name, err)
		}
	}

	log.Println("Daily risk scan completed")
}

// SendReminder 向所有用户发送标准的周五提醒。
func (s *Scheduler) SendReminder() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	users, err := s.feishu.CollectUsers(ctx, s.config.OKRUserIDs, s.config.DepartmentIDs)
	if err != nil {
		log.Printf("Error collecting users for reminder: %v", err)
		return
	}

	log.Printf("Sending OKR update reminder to %d users", len(users))

	msg := "OKR 更新提醒\n\n周末将至，请记得更新本月的 OKR 进展：\n" +
		"- 更新每个 KR 的进度百分比\n" +
		"- 补充本周的进展记录\n" +
		"- 如有阻塞或风险请及时标注\n\n" +
		"保持每周更新，让团队了解你的进展。"

	for _, u := range users {
		name := u.Name
		if name == "" {
			name = u.OpenID
		}
		if err := s.feishu.SendTextMessage(ctx, u.OpenID, msg); err != nil {
			log.Printf("Error sending reminder to %s: %v", name, err)
			continue
		}
		log.Printf("Sent reminder to %s", name)
	}

	log.Println("OKR reminder completed")
}

func timePtr(t time.Time) *time.Time { return &t }
