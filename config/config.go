package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	FeishuAppID     string
	FeishuAppSecret string
	LLMEndpoint string
	LLMAPIKey   string
	LLMModel    string
	SQLitePath      string
	OKRUserIDs      []string
	DepartmentIDs   []string
	CronSchedule    string
	HealthPort      string

	// 风险阈值（天数）
	RiskDaysHigh     int // 多少天未更新视为高风险，默认 14
	RiskDaysCritical int // 多少天未更新视为危急，默认 21

	// 快照保留天数
	SnapshotRetentionDays int // 超过多少天的快照自动清理，默认 90

	// 提醒冷却时间（小时）
	RiskCooldownHighHours     int // 高风险提醒间隔，默认 72（3 天）
	RiskCooldownCriticalHours int // 危急提醒间隔，默认 24（1 天）
}

func Load() (*Config, error) {
	loadEnvFile(".env")

	cfg := &Config{
		FeishuAppID:               getEnv("FEISHU_APP_ID", ""),
		FeishuAppSecret:           getEnv("FEISHU_APP_SECRET", ""),
		LLMEndpoint:               getEnv("LLM_ENDPOINT", "https://api.moonshot.cn/v1"),
		LLMAPIKey:                 getEnv("LLM_API_KEY", ""),
		LLMModel:                  getEnv("LLM_MODEL", "kimi-k2.5"),
		SQLitePath:                getEnv("SQLITE_PATH", "./data/okr-agent.db"),
		CronSchedule:              getEnv("CRON_SCHEDULE", "0 9 * * 1"),
		HealthPort:                getEnv("HEALTH_PORT", "8080"),
		SnapshotRetentionDays:     getEnvInt("SNAPSHOT_RETENTION_DAYS", 90),
		RiskDaysHigh:              getEnvInt("RISK_DAYS_HIGH", 14),
		RiskDaysCritical:          getEnvInt("RISK_DAYS_CRITICAL", 21),
		RiskCooldownHighHours:     getEnvInt("RISK_COOLDOWN_HIGH_HOURS", 72),
		RiskCooldownCriticalHours: getEnvInt("RISK_COOLDOWN_CRITICAL_HOURS", 24),
	}

	if ids := getEnv("OKR_USER_IDS", ""); ids != "" {
		cfg.OKRUserIDs = splitAndTrim(ids, ",")
	}

	if ids := getEnv("FEISHU_DEPARTMENT_IDS", ""); ids != "" {
		cfg.DepartmentIDs = splitAndTrim(ids, ",")
	}

	return cfg, nil
}

// Validate 校验所有必填配置项。
func (c *Config) Validate() error {
	if c.FeishuAppID == "" || c.FeishuAppSecret == "" {
		return fmt.Errorf("FEISHU_APP_ID and FEISHU_APP_SECRET are required")
	}
	if c.LLMAPIKey == "" {
		return fmt.Errorf("LLM_API_KEY is required")
	}
	if len(c.OKRUserIDs) == 0 && len(c.DepartmentIDs) == 0 {
		return fmt.Errorf("at least one of OKR_USER_IDS or FEISHU_DEPARTMENT_IDS is required")
	}
	if c.RiskDaysHigh <= 0 || c.RiskDaysCritical <= 0 {
		return fmt.Errorf("risk threshold days must be positive")
	}
	if c.RiskDaysHigh >= c.RiskDaysCritical {
		return fmt.Errorf("RISK_DAYS_HIGH (%d) must be less than RISK_DAYS_CRITICAL (%d)", c.RiskDaysHigh, c.RiskDaysCritical)
	}
	return nil
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
