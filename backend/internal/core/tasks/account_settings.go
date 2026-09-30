package tasks

import (
	"os"
	"strings"
)

// Account-bound settings never fall back to a process-wide verification code
// or device state: workers execute many different accounts in one process.
type accountTaskSettings struct{ settingsPhone string }

func (s *accountTaskSettings) SetAccountPhone(phone string) {
	s.settingsPhone = strings.TrimSpace(phone)
}

func (s *accountTaskSettings) setting(key string) string {
	return accountTaskSetting(key, s.settingsPhone)
}

func accountTaskSetting(key, phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(key + "_" + phone))
}
