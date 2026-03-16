package scheduler

import (
	"context"
	"log"

	"github.com/robfig/cron/v3"

	"okr-agent/config"
	"okr-agent/evaluator"
	"okr-agent/feishu"
)

// Scheduler manages cron-based OKR checking.
type Scheduler struct {
	cron      *cron.Cron
	feishu    *feishu.Client
	evaluator *evaluator.Evaluator
	config    *config.Config
}

// New creates a new Scheduler.
func New(fc *feishu.Client, eval *evaluator.Evaluator, cfg *config.Config) *Scheduler {
	return &Scheduler{
		cron:      cron.New(),
		feishu:    fc,
		evaluator: eval,
		config:    cfg,
	}
}

// Start begins the cron scheduler.
func (s *Scheduler) Start() error {
	_, err := s.cron.AddFunc(s.config.CronSchedule, func() {
		s.RunCheck()
	})
	if err != nil {
		return err
	}
	s.cron.Start()
	log.Printf("Scheduler started with schedule: %s", s.config.CronSchedule)
	return nil
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// RunCheck performs OKR check for all configured users.
func (s *Scheduler) RunCheck() {
	ctx := context.Background()

	userIDs, err := s.feishu.CollectUserIDs(ctx, s.config.OKRUserIDs, s.config.DepartmentIDs)
	if err != nil {
		log.Printf("Error collecting user IDs: %v", err)
		return
	}

	log.Printf("Starting OKR check for %d users", len(userIDs))

	for _, userID := range userIDs {
		s.checkUser(ctx, userID)
	}

	log.Println("OKR check completed")
}

// CheckSingleUser performs OKR check for a single user and returns the evaluation.
func (s *Scheduler) CheckSingleUser(ctx context.Context, userID string) (string, error) {
	okrData, err := s.feishu.GetUserOKRs(ctx, userID)
	if err != nil {
		return "", err
	}

	okrText := feishu.FormatOKRForEvaluation(okrData)
	evaluation, err := s.evaluator.Evaluate(ctx, okrText)
	if err != nil {
		return "", err
	}

	return evaluation, nil
}

func (s *Scheduler) checkUser(ctx context.Context, userID string) {
	log.Printf("Checking OKR for user: %s", userID)

	evaluation, err := s.CheckSingleUser(ctx, userID)
	if err != nil {
		log.Printf("Error checking user %s: %v", userID, err)
		return
	}

	title := "📊 OKR 周报评价"
	if err := s.feishu.SendRichMessage(ctx, userID, title, evaluation); err != nil {
		log.Printf("Error sending message to %s: %v", userID, err)
		return
	}

	log.Printf("Sent OKR evaluation to user: %s", userID)
}
