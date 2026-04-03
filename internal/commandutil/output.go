package commandutil

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/output"
	"github.com/metorial/cli/internal/terminal"
)

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func WriteValue(writer io.Writer, features terminal.Features, formatInput string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("metorial: failed to encode response: %w", err)
	}

	format, err := output.ParseFormat(formatInput)
	if err != nil {
		return err
	}

	return output.WriteResponse(writer, &fetch.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       body,
	}, output.RenderOptions{
		Format: format,
		Colors: features,
	})
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func Slugify(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = slugPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

func RandomSuffix(length int) string {
	if length <= 0 {
		return ""
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "rand"
	}

	return hex.EncodeToString(bytes)[:length]
}

func MaxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
