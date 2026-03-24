package agent

import "os"

var envVarAgents = map[string]string{
	"ANTIGRAVITY_CLI_ALIAS":          "antigravity",
	"CLAUDECODE":                     "claude_code",
	"CLINE_ACTIVE":                   "cline",
	"CODEX_SANDBOX":                  "codex_cli",
	"CODEX_THREAD_ID":                "codex_cli",
	"CODEX_SANDBOX_NETWORK_DISABLED": "codex_cli",
	"CODEX_CI":                       "codex_cli",
	"CURSOR_AGENT":                   "cursor",
	"GEMINI_CLI":                     "gemini_cli",
	"OPENCODE":                       "open_code",
}

func IsAiAgent() (bool, string) {
	for envVar, agentName := range envVarAgents {
		if os.Getenv(envVar) != "" {
			return true, agentName
		}
	}

	return false, ""
}
