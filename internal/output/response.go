package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/metorial/cli/internal/fetch"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type RenderOptions struct {
	Format  Format
	Include bool
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
		return writeStructured(writer, response.Body)
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

func writeStructured(writer io.Writer, body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return writeRaw(writer, body)
	}

	switch typed := value.(type) {
	case map[string]any:
		return writeStructuredObject(writer, typed)
	case []any:
		return writeStructuredArray(writer, typed)
	default:
		return writeRaw(writer, trimmed)
	}
}

func writeStructuredObject(writer io.Writer, value map[string]any) error {
	if items, ok := value["items"].([]any); ok {
		metaItems := mapToDataList(omitKeys(value, "items"))
		if len(metaItems) > 0 {
			if err := RenderDataList(writer, metaItems); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(writer); err != nil {
				return err
			}
		}

		return writeStructuredArray(writer, items)
	}

	return RenderDataList(writer, mapToDataList(value))
}

func writeStructuredArray(writer io.Writer, items []any) error {
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
			Columns: []string{"Value"},
			Rows:    make([][]string, 0, len(items)),
		}
		for _, item := range items {
			table.Rows = append(table.Rows, []string{renderInlineValue(item)})
		}
		return table.Render(writer)
	}

	columns := collectColumns(rows)
	table := Table{
		Columns: make([]string, 0, len(columns)),
		Rows:    make([][]string, 0, len(rows)),
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

func mapToDataList(value map[string]any) []DataListItem {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]DataListItem, 0, len(keys))
	for _, key := range keys {
		items = append(items, DataListItem{
			Label: humanize(key),
			Value: renderInlineValue(value[key]),
		})
	}

	return items
}

func omitKeys(value map[string]any, keys ...string) map[string]any {
	excluded := map[string]struct{}{}
	for _, key := range keys {
		excluded[key] = struct{}{}
	}

	result := map[string]any{}
	for key, entry := range value {
		if _, ok := excluded[key]; ok {
			continue
		}
		result[key] = entry
	}

	return result
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
