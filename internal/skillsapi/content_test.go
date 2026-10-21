package skillsapi_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/skillsapi"
)

func TestIsTextValidatesMarkdownDocuments(t *testing.T) {
	if _, err := skillsapi.IsText("/notes.md", []byte("caf\xe9")); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 error = %v", err)
	}
	if _, err := skillsapi.IsText("/big.md", bytes.Repeat([]byte("a"), skillsapi.MaxTextBytes+1)); err == nil {
		t.Fatal("IsText() should reject documents larger than MaxTextBytes")
	}

	if isText, err := skillsapi.IsText("/notes.md", []byte("# Notes")); err != nil || !isText {
		t.Fatalf("IsText(notes.md) = %v, %v", isText, err)
	}
	if isText, err := skillsapi.IsText("/logo.png", []byte("\x89PNG\x00")); err != nil || isText {
		t.Fatalf("IsText(logo.png) = %v, %v", isText, err)
	}
}
