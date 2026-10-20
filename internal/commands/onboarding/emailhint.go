package onboarding

import (
	"context"
	"net/mail"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type emailHint struct {
	Email  string
	Source string
}

const emailHintTimeout = 2 * time.Second

var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

var runHintCommand = func(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), emailHintTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, name, args...).Output()
	return string(output), err
}

var maskingEmailDomains = []string{
	"privaterelay.appleid.com",
	"mozmail.com",
	"relay.firefox.com",
	"duck.com",
	"simplelogin.com",
	"simplelogin.co",
	"simplelogin.fr",
	"simplelogin.io",
	"slmail.me",
	"aleeas.com",
	"8alias.com",
	"anonaddy.com",
	"anonaddy.me",
	"addy.io",
	"passmail.net",
	"passmail.com",
	"passinbox.com",
	"passfwd.com",
	"cloaked.id",
	"spamgourmet.com",
	"33mail.com",
}

func isMaskedEmail(email string) bool {
	lower := strings.ToLower(email)
	if strings.Contains(lower, "noreply") || strings.Contains(lower, "no-reply") {
		return true
	}

	at := strings.LastIndex(lower, "@")
	if at < 0 {
		return true
	}

	domain := lower[at+1:]
	for _, masked := range maskingEmailDomains {
		if domain == masked || strings.HasSuffix(domain, "."+masked) {
			return true
		}
	}
	return false
}

func usableHintEmail(candidate string) (string, bool) {
	address, err := mail.ParseAddress(strings.TrimSpace(candidate))
	if err != nil {
		return "", false
	}

	if isMaskedEmail(address.Address) {
		return "", false
	}

	return address.Address, true
}

func emailFromGit() *emailHint {
	output, err := runHintCommand("git", "config", "--get", "user.email")
	if err != nil {
		return nil
	}

	if email, ok := usableHintEmail(output); ok {
		return &emailHint{Email: email, Source: "git config user.email"}
	}
	return nil
}

func emailFromClaudeCode() *emailHint {
	output, err := runHintCommand("claude", "auth", "status")
	if err != nil {
		return nil
	}

	if email, ok := usableHintEmail(emailPattern.FindString(output)); ok {
		return &emailHint{Email: email, Source: "`claude auth status`"}
	}
	return nil
}

// The result is only a suggestion for the agent to confirm with the user; it is never sent to Metorial.
func detectEmailHint(agentName string) *emailHint {
	if hint := emailFromGit(); hint != nil {
		return hint
	}

	if agentName == "claude_code" {
		return emailFromClaudeCode()
	}

	return nil
}
