package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/metorial/cli/internal/fetch"
	"github.com/metorial/cli/internal/terminal"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type RenderOptions struct {
	Format  Format
	Include bool
	Colors  terminal.Features
}

func WriteResponse(writer io.Writer, response *fetch.Response, options RenderOptions) error {
	if response == nil {
		return nil
	}

	if options.Include {
		if err := writeResponseMeta(writer, response); err != nil {
			return err
		}
	}

	switch options.Format {
	case FormatJSON:
		return writeJSON(writer, response.Body)
	case FormatYAML:
		return writeYAML(writer, response.Body)
	case FormatTOML:
		return writeTOML(writer, response.Body)
	case FormatStructured:
		return writeStructured(writer, response.Body, options.Colors)
	default:
		return fmt.Errorf("metorial: unsupported output format %q", options.Format)
	}
}

func writeResponseMeta(writer io.Writer, response *fetch.Response) error {
	items := []DataListItem{
		{Label: "Status", Value: response.Status},
	}

	headerKeys := sortedKeys(response.Headers)
	for _, key := range headerKeys {
		items = append(items, DataListItem{
			Label: key,
			Value: strings.Join(response.Headers[key], ", "),
		})
	}

	if err := RenderDataList(writer, items); err != nil {
		return err
	}

	_, err := fmt.Fprintln(writer)
	return err
}

func writeYAML(writer io.Writer, body []byte) error {
	value, raw, err := decodeBody(body)
	if err != nil {
		return writeRaw(writer, body)
	}
	if value == nil {
		return nil
	}

	formatted, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("metorial: failed to format YAML response: %w", err)
	}

	if bytes.Equal(bytes.TrimSpace(formatted), bytes.TrimSpace(raw)) {
		return writeRaw(writer, raw)
	}

	return writeRaw(writer, formatted)
}

func writeTOML(writer io.Writer, body []byte) error {
	value, raw, err := decodeBody(body)
	if err != nil {
		return writeRaw(writer, body)
	}
	if value == nil {
		return nil
	}

	formatted, err := toml.Marshal(value)
	if err != nil {
		return fmt.Errorf("metorial: failed to format TOML response: %w", err)
	}

	if bytes.Equal(bytes.TrimSpace(formatted), bytes.TrimSpace(raw)) {
		return writeRaw(writer, raw)
	}

	return writeRaw(writer, formatted)
}

func decodeBody(body []byte) (any, []byte, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil, nil
	}

	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, body, err
	}

	return value, bytes.TrimSpace(body), nil
}

func writeJSON(writer io.Writer, body []byte) error {
	value, raw, err := decodeBody(body)
	if err != nil {
		return writeRaw(writer, body)
	}
	if value == nil {
		return nil
	}

	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("metorial: failed to format JSON response: %w", err)
	}

	if bytes.Equal(bytes.TrimSpace(formatted), bytes.TrimSpace(raw)) {
		return writeRaw(writer, raw)
	}

	return writeRaw(writer, formatted)
}

func writeStructured(writer io.Writer, body []byte, features terminal.Features) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return writeRaw(writer, body)
	}

	colors := terminal.NewColorizer(features)

	switch typed := value.(type) {
	case map[string]any:
		return writeStructuredObject(writer, typed, colors, features)
	case []any:
		return writeStructuredArray(writer, typed, features)
	default:
		return writeRaw(writer, trimmed)
	}
}

func writeStructuredObject(writer io.Writer, value map[string]any, colors terminal.Colorizer, features terminal.Features) error {
	if items, ok := value["items"].([]any); ok {
		return writeStructuredArray(writer, items, features)
	}

	return renderStructuredDetail(writer, value, 0, objectTitle(value), colors)
}

func writeStructuredArray(writer io.Writer, items []any, features terminal.Features) error {
	if len(items) == 0 {
		_, err := fmt.Fprintln(writer, "(no items)")
		return err
	}

	rows := make([]map[string]any, 0, len(items))
	allObjects := true
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			allObjects = false
			break
		}
		rows = append(rows, object)
	}

	if !allObjects {
		table := Table{
			Columns:  []string{"Value"},
			Rows:     make([][]string, 0, len(items)),
			Features: features,
		}
		for _, item := range items {
			table.Rows = append(table.Rows, []string{renderInlineValue(item)})
		}
		return table.Render(writer)
	}

	columns := preferredColumns(rows)
	table := Table{
		Columns:  make([]string, 0, len(columns)),
		Rows:     make([][]string, 0, len(rows)),
		Features: features,
		MaxWidth: features.Width,
	}

	for _, column := range columns {
		table.Columns = append(table.Columns, humanize(column))
	}

	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, renderInlineValue(row[column]))
		}
		table.Rows = append(table.Rows, values)
	}

	return table.Render(writer)
}

func renderStructuredDetail(writer io.Writer, value map[string]any, depth int, title string, colors terminal.Colorizer) error {
	indent := strings.Repeat("  ", depth)
	if strings.TrimSpace(title) != "" {
		if _, err := fmt.Fprintf(writer, "%s%s\n\n", indent, colors.Bold(title)); err != nil {
			return err
		}
	}

	for _, key := range orderedKeys(value) {
		entry := value[key]
		switch typed := entry.(type) {
		case map[string]any:
			sectionTitle := fmt.Sprintf("%s%s", strings.Repeat("  ", depth), colors.Accent(humanize(key)))
			if _, err := fmt.Fprintf(writer, "%s\n", sectionTitle); err != nil {
				return err
			}
			if err := renderStructuredDetail(writer, typed, depth+1, "", colors); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(writer); err != nil {
				return err
			}
		case []any:
			if hasObjectItems(typed) {
				sectionTitle := fmt.Sprintf("%s%s", strings.Repeat("  ", depth), colors.Accent(humanize(key)))
				if _, err := fmt.Fprintf(writer, "%s\n", sectionTitle); err != nil {
					return err
				}
				for index, item := range typed {
					object, _ := item.(map[string]any)
					itemTitle := fmt.Sprintf("%s[%d]", humanize(key), index+1)
					if named, ok := object["name"].(string); ok && strings.TrimSpace(named) != "" {
						itemTitle = named
					}
					if err := renderStructuredDetail(writer, object, depth+1, itemTitle, colors); err != nil {
						return err
					}
					if _, err := fmt.Fprintln(writer); err != nil {
						return err
					}
				}
				continue
			}

			if _, err := fmt.Fprintf(writer, "%s%s = %s\n", indent, key, renderInlineValue(entry)); err != nil {
				return err
			}
		default:
			if isEmptyValue(entry) {
				continue
			}
			if _, err := fmt.Fprintf(writer, "%s%s = %s\n", indent, key, renderDetailValue(entry)); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeRaw(writer io.Writer, body []byte) error {
	if len(body) == 0 {
		return nil
	}

	if _, err := writer.Write(body); err != nil {
		return err
	}

	if body[len(body)-1] != '\n' {
		_, err := fmt.Fprintln(writer)
		return err
	}

	return nil
}

func collectColumns(rows []map[string]any) []string {
	columnSet := map[string]struct{}{}
	for _, row := range rows {
		for key := range row {
			columnSet[key] = struct{}{}
		}
	}

	columns := make([]string, 0, len(columnSet))
	for key := range columnSet {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return columns
}

func preferredColumns(rows []map[string]any) []string {
	preferred := []string{"id", "name", "slug", "description", "created_at"}
	columns := make([]string, 0, len(preferred))
	for _, key := range preferred {
		if rowsHaveKey(rows, key) {
			columns = append(columns, key)
		}
	}
	if len(columns) > 0 {
		return columns
	}
	return collectColumns(rows)
}

func rowsHaveKey(rows []map[string]any, key string) bool {
	for _, row := range rows {
		if _, ok := row[key]; ok {
			return true
		}
	}
	return false
}

func sortedKeys(value map[string][]string) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func renderInlineValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool, float64, int:
		return fmt.Sprintf("%v", typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, renderInlineValue(item))
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		formatted, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(formatted)
	default:
		formatted, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(formatted)
	}
}

func renderDetailValue(value any) string {
	switch typed := value.(type) {
	case string:
		return fmt.Sprintf("%q", typed)
	default:
		return renderInlineValue(value)
	}
}

func orderedKeys(value map[string]any) []string {
	priority := []string{"object", "id", "name", "slug", "description", "status", "access", "created_at", "updated_at"}
	keys := make([]string, 0, len(value))
	seen := map[string]struct{}{}
	for _, key := range priority {
		if _, ok := value[key]; ok {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}
	rest := make([]string, 0, len(value))
	for key := range value {
		if _, ok := seen[key]; ok {
			continue
		}
		rest = append(rest, key)
	}
	sort.Strings(rest)
	keys = append(keys, rest...)
	return keys
}

func objectTitle(value map[string]any) string {
	object, _ := value["object"].(string)
	if object == "" {
		return colorsafeTitle("Record")
	}
	return colorsafeTitle(humanize(object))
}

func colorsafeTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, ":") {
		return value
	}
	return value + ":"
}

func hasObjectItems(values []any) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if _, ok := value.(map[string]any); !ok {
			return false
		}
	}
	return true
}

func isEmptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func humanize(value string) string {
	parts := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(value))
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}

	return strings.Join(parts, " ")
}
