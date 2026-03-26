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

// Scheduler manages cron-based OKR checking with agent-driven intelligence.
type Scheduler struct {
	cron   *cron.Cron
	feishu *feishu.Client
	agent  *agent.Agent
	store  *memory.Store
	config *config.Config
}

// New creates a new Scheduler.
func New(fc *feishu.Client, ag *agent.Agent, store *memory.Store, cfg *config.Config) *Scheduler {
	return &Scheduler{
		cron:   cron.New(),
		feishu: fc,
		agent:  ag,
		store:  store,
		config: cfg,
	}
}

// Start begins the cron scheduler.
func (s *Scheduler) Start() error {
	// Main OKR check (configurable, default: Monday 9:00)
	_, err := s.cron.AddFunc(s.config.CronSchedule, func() {
		s.RunCheck()
	})
	if err != nil {
		return err
	}

	// Daily risk scan at 10:00
	_, err = s.cron.AddFunc("0 10 * * *", func() {
		s.DailyRiskScan()
	})
	if err != nil {
		return err
	}

	// Friday reminder at 10:00
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

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// RunCheck performs agent-driven OKR evaluation for all configured users.
func (s *Scheduler) RunCheck() {
	ctx := context.Background()

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

// DailyRiskScan checks all users' OKR update times and triggers personalized reminders.
func (s *Scheduler) DailyRiskScan() {
	ctx := context.Background()

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

		// Fetch OKR data to check last update time
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
			daysSinceUpdate = 999 // never updated
		}

		// Determine risk level
		riskLevel := "normal"
		switch {
		case daysSinceUpdate >= 21:
			riskLevel = "critical"
		case daysSinceUpdate >= 14:
			riskLevel = "high"
		case daysSinceUpdate >= 7:
			riskLevel = "normal"
		}

		// Save state
		state := &memory.SchedulerState{
			UserID:          u.OpenID,
			RiskLevel:       riskLevel,
			DaysSinceUpdate: daysSinceUpdate,
		}

		// Check if we should send a reminder
		existingState, _ := s.store.GetSchedulerState(ctx, u.OpenID)
		shouldRemind := false

		switch riskLevel {
		case "critical":
			// Always remind for critical
			if existingState.LastReminder == nil || time.Since(*existingState.LastReminder) > 24*time.Hour {
				shouldRemind = true
			}
		case "high":
			// Remind if not reminded in last 3 days
			if existingState.LastReminder == nil || time.Since(*existingState.LastReminder) > 3*24*time.Hour {
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
				_ = s.store.UpdateReminderTime(ctx, u.OpenID)
			}
		}

		_ = s.store.SaveSchedulerState(ctx, state)
	}

	log.Println("Daily risk scan completed")
}

// SendReminder sends a standard Friday reminder to all users.
func (s *Scheduler) SendReminder() {
	ctx := context.Background()

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
