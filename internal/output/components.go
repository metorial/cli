package output

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type Table struct {
	Columns []string
	Rows    [][]string
}

func (t Table) Render(writer io.Writer) error {
	if len(t.Columns) == 0 {
		return nil
	}

	widths := make([]int, len(t.Columns))
	for index, column := range t.Columns {
		widths[index] = runeWidth(column)
	}

	for _, row := range t.Rows {
		for index, value := range row {
			if index >= len(widths) {
				continue
			}
			widths[index] = max(widths[index], runeWidth(value))
		}
	}

	if _, err := fmt.Fprintln(writer, formatTableRow(t.Columns, widths)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(writer, formatTableDivider(widths)); err != nil {
		return err
	}

	for _, row := range t.Rows {
		if _, err := fmt.Fprintln(writer, formatTableRow(row, widths)); err != nil {
			return err
		}
	}

	return nil
}

type DataListItem struct {
	Label string
	Value string
}

func RenderDataList(writer io.Writer, items []DataListItem) error {
	if len(items) == 0 {
		return nil
	}

	maxLabelWidth := 0
	for _, item := range items {
		maxLabelWidth = max(maxLabelWidth, runeWidth(item.Label))
	}

	for _, item := range items {
		if _, err := fmt.Fprintf(writer, "%-*s  %s\n", maxLabelWidth, item.Label, item.Value); err != nil {
			return err
		}
	}

	return nil
}

func formatTableRow(values []string, widths []int) string {
	cells := make([]string, len(widths))
	for index := range widths {
		value := ""
		if index < len(values) {
			value = values[index]
		}
		cells[index] = padRight(value, widths[index])
	}

	return "  " + strings.Join(cells, "  ")
}

func formatTableDivider(widths []int) string {
	parts := make([]string, len(widths))
	for index, width := range widths {
		parts[index] = strings.Repeat("-", width)
	}

	return "  " + strings.Join(parts, "  ")
}

func padRight(value string, width int) string {
	padding := width - runeWidth(value)
	if padding <= 0 {
		return value
	}

	return value + strings.Repeat(" ", padding)
}

func runeWidth(value string) int {
	return utf8.RuneCountInString(value)
}

func max(left, right int) int {
	if left > right {
		return left
	}

	return right
}
