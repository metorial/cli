package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/terminal"
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
	if defaultFormat != FormatStructured {
		t.Fatalf("ParseFormat(\"\") = %q, want %q", defaultFormat, FormatStructured)
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
	if strings.Contains(rendered, "Pagination") {
		t.Fatalf("WriteResponse() should not include pagination metadata in structured list view: %q", rendered)
	}
	if !strings.Contains(rendered, "GitHub") || !strings.Contains(rendered, "Slack") {
		t.Fatalf("WriteResponse() missing row values: %q", rendered)
	}
}

func TestTableRenderWrapsToTerminalWidth(t *testing.T) {
	t.Parallel()

	table := Table{
		Columns:  []string{"Id", "Description"},
		Rows:     [][]string{{"prov_1", "Search the web and fetch readable content from selected pages built for fast discovery and extraction."}},
		Features: terminal.Features{Width: 48, HasColor: false},
		MaxWidth: 48,
	}

	var output bytes.Buffer
	if err := table.Render(&output); err != nil {
		t.Fatalf("Table.Render() error = %v", err)
	}

	lines := strings.Split(strings.TrimRight(output.String(), "\n"), "\n")
	for _, line := range lines {
		if len(line) > 48 {
			t.Fatalf("table line exceeds width: %q", line)
		}
	}
	if len(lines) < 4 {
		t.Fatalf("expected wrapped multi-line table output, got %q", output.String())
	}
}

func TestWriteStructuredGetResponseUsesDetailLayout(t *testing.T) {
	t.Parallel()

	response := &fetch.Response{
		Status:  "200 OK",
		Headers: map[string][]string{},
		Body: []byte(`{
			"object":"provider",
			"id":"prov_1",
			"name":"Metorial Search",
			"slug":"metorial-search",
			"description":"Search the web",
			"status":"active",
			"created_at":"2026-03-09T12:55:56.116Z",
			"updated_at":"2026-04-01T13:30:46.639Z",
			"publisher":{"object":"provider.publisher","id":"pub_1","name":"Metorial"}
		}`),
	}

	var output bytes.Buffer
	err := WriteResponse(&output, response, RenderOptions{Format: FormatStructured})
	if err != nil {
		t.Fatalf("WriteResponse() error = %v", err)
	}

	rendered := output.String()
	if !strings.Contains(rendered, "Provider:") {
		t.Fatalf("WriteResponse() missing heading: %q", rendered)
	}
	if !strings.Contains(rendered, "id = \"prov_1\"") {
		t.Fatalf("WriteResponse() missing scalar field: %q", rendered)
	}
	if !strings.Contains(rendered, "Publisher") {
		t.Fatalf("WriteResponse() missing nested section: %q", rendered)
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

func TestRenderBox(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := RenderBox(&output, []string{
		"https://app.metorial.com/cli/auth",
		"Verification Code: ABCD-1234",
	}, BoxOptions{})
	if err != nil {
		t.Fatalf("RenderBox() error = %v", err)
	}

	rendered := output.String()
	if !strings.Contains(rendered, "+------------------------------------+") {
		t.Fatalf("RenderBox() missing border: %q", rendered)
	}
	if !strings.Contains(rendered, "| https://app.metorial.com/cli/auth  |") {
		t.Fatalf("RenderBox() missing url line: %q", rendered)
	}
	if !strings.Contains(rendered, "| Verification Code: ABCD-1234       |") {
		t.Fatalf("RenderBox() missing code line: %q", rendered)
	}
}

func TestRenderUnicodeBoxWrapsLongContent(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := RenderBox(&output, []string{
		"https://app.metorial.com/cli/auth/very/long/path",
	}, BoxOptions{
		MaxWidth: 24,
		Unicode:  true,
	})
	if err != nil {
		t.Fatalf("RenderBox() error = %v", err)
	}

	rendered := output.String()
	if !strings.Contains(rendered, "╭") || !strings.Contains(rendered, "╰") {
		t.Fatalf("RenderBox() missing unicode borders: %q", rendered)
	}
	if !strings.Contains(rendered, "https://app.metorial") {
		t.Fatalf("RenderBox() missing wrapped content: %q", rendered)
	}
	if !strings.Contains(rendered, ".com/cli/auth/very/l") {
		t.Fatalf("RenderBox() missing overflow line: %q", rendered)
	}
}
