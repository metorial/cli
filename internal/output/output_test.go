package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/fetch"
)

func TestParseFormat(t *testing.T) {
	t.Parallel()

	format, err := ParseFormat("json")
	if err != nil {
		t.Fatalf("ParseFormat() error = %v", err)
	}
	if format != FormatJSON {
		t.Fatalf("ParseFormat() = %q, want %q", format, FormatJSON)
	}

	defaultFormat, err := ParseFormat("")
	if err != nil {
		t.Fatalf("ParseFormat(\"\") error = %v", err)
	}
	if defaultFormat != FormatYAML {
		t.Fatalf("ParseFormat(\"\") = %q, want %q", defaultFormat, FormatYAML)
	}
}

func TestWriteStructuredItemsResponse(t *testing.T) {
	t.Parallel()

	response := &fetch.Response{
		Status:  "200 OK",
		Headers: map[string][]string{},
		Body: []byte(`{
			"items": [
				{"id":"prov_1","name":"GitHub"},
				{"id":"prov_2","name":"Slack"}
			],
			"pagination": {"has_more_after": false}
		}`),
	}

	var output bytes.Buffer
	err := WriteResponse(&output, response, RenderOptions{Format: FormatStructured})
	if err != nil {
		t.Fatalf("WriteResponse() error = %v", err)
	}

	rendered := output.String()
	if !strings.Contains(rendered, "Id") || !strings.Contains(rendered, "Name") {
		t.Fatalf("WriteResponse() missing table headers: %q", rendered)
	}
	if !strings.Contains(rendered, "GitHub") || !strings.Contains(rendered, "Slack") {
		t.Fatalf("WriteResponse() missing row values: %q", rendered)
	}
	if !strings.Contains(rendered, "Pagination") {
		t.Fatalf("WriteResponse() missing data list metadata: %q", rendered)
	}
}

func TestWriteYAMLResponse(t *testing.T) {
	t.Parallel()

	response := &fetch.Response{
		Status:  "200 OK",
		Headers: map[string][]string{},
		Body:    []byte(`{"id":"prov_1","name":"GitHub"}`),
	}

	var output bytes.Buffer
	err := WriteResponse(&output, response, RenderOptions{Format: FormatYAML})
	if err != nil {
		t.Fatalf("WriteResponse() error = %v", err)
	}

	rendered := output.String()
	if !strings.Contains(rendered, "id: prov_1") || !strings.Contains(rendered, "name: GitHub") {
		t.Fatalf("WriteResponse() missing yaml values: %q", rendered)
	}
}

func TestWriteTOMLResponse(t *testing.T) {
	t.Parallel()

	response := &fetch.Response{
		Status:  "200 OK",
		Headers: map[string][]string{},
		Body:    []byte(`{"id":"prov_1","name":"GitHub"}`),
	}

	var output bytes.Buffer
	err := WriteResponse(&output, response, RenderOptions{Format: FormatTOML})
	if err != nil {
		t.Fatalf("WriteResponse() error = %v", err)
	}

	rendered := output.String()
	if !strings.Contains(rendered, "id = 'prov_1'") || !strings.Contains(rendered, "name = 'GitHub'") {
		t.Fatalf("WriteResponse() missing toml values: %q", rendered)
	}
}
