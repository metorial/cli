package output

import (
	"fmt"
	"strings"
)

type Format string

const (
	FormatStructured Format = "structured"
	FormatJSON       Format = "json"
	FormatYAML       Format = "yaml"
	FormatTOML       Format = "toml"
)

func ParseFormat(raw string) (Format, error) {
	value := Format(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return FormatYAML, nil
	}

	switch value {
	case FormatStructured, FormatJSON, FormatYAML, FormatTOML:
		return value, nil
	default:
		return "", fmt.Errorf("metorial: invalid format %q, expected one of: yaml, toml, json, structured", raw)
	}
}
