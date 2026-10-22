package skillsapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const IfMatchAbsent = "absent"

const MaxTextBytes = 5 * 1024 * 1024

var documentExtensions = map[string]bool{".md": true, ".markdown": true}

func IsText(filePath string, content []byte) (bool, error) {
	if !documentExtensions[strings.ToLower(path.Ext(filePath))] {
		return len(content) <= MaxTextBytes && utf8.Valid(content) && !bytes.Contains(content, []byte{0}), nil
	}

	if !utf8.Valid(content) {
		return false, fmt.Errorf("metorial: %s is not valid UTF-8; Markdown files become documents and must be UTF-8 text", filePath)
	}
	if len(content) > MaxTextBytes {
		return false, fmt.Errorf("metorial: %s is larger than %d MB, the limit for Markdown documents", filePath, MaxTextBytes/1024/1024)
	}
	return true, nil
}

// The document store strips trailing newlines.
func RestoreTrailingNewline(kind string, content []byte) []byte {
	if kind == FileKindDocument && len(content) > 0 && content[len(content)-1] != '\n' {
		return append(content, '\n')
	}
	return content
}

func MatchEtag(etag string) json.RawMessage {
	encoded, _ := json.Marshal(etag)
	return encoded
}

func MatchAbsent() json.RawMessage {
	return json.RawMessage("null")
}

func ParseIfMatch(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	switch value {
	case "":
		return nil
	case IfMatchAbsent:
		return MatchAbsent()
	}
	return MatchEtag(value)
}

func ParseEtag(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == IfMatchAbsent {
		return "", errors.New(`metorial: if-match "absent" only applies to writes; pass the etag of the file instead`)
	}
	return value, nil
}
