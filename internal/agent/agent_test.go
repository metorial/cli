package agent

import "testing"

func TestIsAiAgent(t *testing.T) {
	tests := []struct {
		name      string
		envVar    string
		envValue  string
		wantOK    bool
		wantAgent string
	}{
		{name: "no env", wantOK: false, wantAgent: ""},
		{name: "antigravity", envVar: "ANTIGRAVITY_CLI_ALIAS", envValue: "1", wantOK: true, wantAgent: "antigravity"},
		{name: "claude code", envVar: "CLAUDECODE", envValue: "1", wantOK: true, wantAgent: "claude_code"},
		{name: "cline", envVar: "CLINE_ACTIVE", envValue: "1", wantOK: true, wantAgent: "cline"},
		{name: "codex sandbox", envVar: "CODEX_SANDBOX", envValue: "1", wantOK: true, wantAgent: "codex_cli"},
		{name: "codex thread", envVar: "CODEX_THREAD_ID", envValue: "1", wantOK: true, wantAgent: "codex_cli"},
		{name: "codex network disabled", envVar: "CODEX_SANDBOX_NETWORK_DISABLED", envValue: "1", wantOK: true, wantAgent: "codex_cli"},
		{name: "codex ci", envVar: "CODEX_CI", envValue: "1", wantOK: true, wantAgent: "codex_cli"},
		{name: "cursor", envVar: "CURSOR_AGENT", envValue: "1", wantOK: true, wantAgent: "cursor"},
		{name: "gemini", envVar: "GEMINI_CLI", envValue: "1", wantOK: true, wantAgent: "gemini_cli"},
		{name: "open code", envVar: "OPENCODE", envValue: "1", wantOK: true, wantAgent: "open_code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for envVar := range envVarAgents {
				t.Setenv(envVar, "")
			}

			if tt.envVar != "" {
				t.Setenv(tt.envVar, tt.envValue)
			}

			gotOK, gotAgent := IsAiAgent()
			if gotOK != tt.wantOK {
				t.Fatalf("IsAiAgent() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotAgent != tt.wantAgent {
				t.Fatalf("IsAiAgent() agent = %q, want %q", gotAgent, tt.wantAgent)
			}
		})
	}
}
