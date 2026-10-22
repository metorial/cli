package commandutil

import "testing"

func TestShellQuote(t *testing.T) {
	for value, want := range map[string]string{
		"":                  "''",
		"release-notes":     "release-notes",
		"./skills/a_b@1.0":  "./skills/a_b@1.0",
		"key=value,x:y+z%":  "key=value,x:y+z%",
		"my skill":          "'my skill'",
		"it's":              `'it'"'"'s'`,
		"$HOME":             "'$HOME'",
		"a;rm -rf /":        "'a;rm -rf /'",
		"`id`":              "'`id`'",
		"*.md":              "'*.md'",
		"~/skills":          "'~/skills'",
		"a&b|c>d<e(f)!g#h{": "'a&b|c>d<e(f)!g#h{'",
	} {
		if got := ShellQuote(value); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", value, got, want)
		}
	}
}
