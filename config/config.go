package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	FeishuAppID       string
	FeishuAppSecret   string
	AnthropicAPIKey   string
	OKRUserIDs        []string
	DepartmentIDs     []string
	CronSchedule      string
}

func Load() (*Config, error) {
	// Load .env file if it exists
	loadEnvFile(".env")

	cfg := &Config{
		FeishuAppID:     getEnv("FEISHU_APP_ID", ""),
		FeishuAppSecret: getEnv("FEISHU_APP_SECRET", ""),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		CronSchedule:    getEnv("CRON_SCHEDULE", "0 9 * * 1"),
	}

	if ids := getEnv("OKR_USER_IDS", ""); ids != "" {
		cfg.OKRUserIDs = splitAndTrim(ids, ",")
	}

	if ids := getEnv("FEISHU_DEPARTMENT_IDS", ""); ids != "" {
		cfg.DepartmentIDs = splitAndTrim(ids, ",")
	}

	return cfg, nil
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
		// Don't override existing env vars
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
