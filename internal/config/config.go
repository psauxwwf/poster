package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TelegramBotToken string
	TelegramAdminID  string
	Proxychains      bool
}

func Load(path string) (Config, error) {
	if err := loadDotEnv(path); err != nil {
		return Config{}, err
	}

	return Config{
		TelegramBotToken: toStr(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramAdminID:  toStr(os.Getenv("TELEGRAM_ADMIN_ID")),
		Proxychains:      toBool("PROXYCHAINS"),
	}, nil
}

func loadDotEnv(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}

	return nil
}

func toStr(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, `"`+"'")
}

func toBool(key string) bool {
	value := toStr(os.Getenv(key))
	if value == "" {
		return false
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}

	return parsed
}
