package onboarding

import (
	"os"
	"os/user"
	"strings"
)

var currentUsername = func() string {
	if current, err := user.Current(); err == nil && strings.TrimSpace(current.Username) != "" {
		return current.Username
	}
	return firstEnv("USER", "USERNAME")
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func workspaceName() string {
	username := strings.TrimSpace(currentUsername())

	// Windows reports DOMAIN\user.
	if index := strings.LastIndex(username, `\`); index >= 0 {
		username = username[index+1:]
	}

	if username == "" {
		return ""
	}
	return username + "'s workspace"
}
