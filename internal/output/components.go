package output

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/metorial/cli/internal/terminal"
)

var ansiPattern = regexp.MustCompile(`\x1b(?:\[[0-9;]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

const defaultTableWidth = 100

type BoxOptions struct {
	MaxWidth int
	Unicode  bool
}

type Table struct {
	Columns  []string
	Rows     [][]string
	Features terminal.Features
	MaxWidth int
}

func (t Table) Render(writer io.Writer) error {
	if len(t.Columns) == 0 {
		return nil
	}

	colors := terminal.NewColorizer(t.Features)
	widths := t.resolveColumnWidths()
	header := make([]string, len(t.Columns))
	for index, column := range t.Columns {
		if stripANSI(column) != column {
			header[index] = column
			continue
		}
		header[index] = colors.Accent(column)
	}

	for _, line := range formatTableLines(header, widths) {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(writer, colors.Muted(formatTableDivider(widths))); err != nil {
		return err
	}
	for _, row := range t.Rows {
		for _, line := range formatTableLines(row, widths) {
			if _, err := fmt.Fprintln(writer, line); err != nil {
				return err
			}
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

func RenderBox(writer io.Writer, lines []string, options BoxOptions) error {
	if len(lines) == 0 {
		return nil
	}

	innerWidth := 0
	for _, line := range lines {
		innerWidth = max(innerWidth, runeWidth(line))
	}

	targetWidth := innerWidth + 5
	if options.MaxWidth > 0 {
		targetWidth = min(options.MaxWidth, targetWidth)
	}

	if targetWidth < 7 {
		targetWidth = 7
	}

	contentWidth := max(1, targetWidth-4)
	wrappedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		wrappedLines = append(wrappedLines, wrapLine(line, contentWidth)...)
	}

	if len(wrappedLines) == 0 {
		wrappedLines = []string{""}
	}

	box := asciiBox()
	if options.Unicode {
		box = unicodeBox()
	}

	border := box.topLeft + strings.Repeat(box.horizontal, contentWidth+2) + box.topRight
	if _, err := fmt.Fprintln(writer, border); err != nil {
		return err
	}

	for _, line := range wrappedLines {
		if _, err := fmt.Fprintf(writer, "%s %s %s\n", box.vertical, padRight(line, contentWidth), box.vertical); err != nil {
			return err
		}
	}

	border = box.bottomLeft + strings.Repeat(box.horizontal, contentWidth+2) + box.bottomRight
	if _, err := fmt.Fprintln(writer, border); err != nil {
		return err
	}

	return nil
}

type boxChars struct {
	topLeft     string
	topRight    string
	bottomLeft  string
	bottomRight string
	horizontal  string
	vertical    string
}

func asciiBox() boxChars {
	return boxChars{
		topLeft:     "+",
		topRight:    "+",
		bottomLeft:  "+",
		bottomRight: "+",
		horizontal:  "-",
		vertical:    "|",
	}
}

func unicodeBox() boxChars {
	return boxChars{
		topLeft:     "╭",
		topRight:    "╮",
		bottomLeft:  "╰",
		bottomRight: "╯",
		horizontal:  "─",
		vertical:    "│",
	}
}

func wrapLine(value string, width int) []string {
	if width <= 0 {
		return []string{value}
	}

	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")
	wrapped := make([]string, 0, len(rawLines))
	for _, rawLine := range rawLines {
		if rawLine == "" {
			wrapped = append(wrapped, "")
			continue
		}

		for runeWidth(rawLine) > width {
			splitAt := bestWrapIndex(rawLine, width)
			wrapped = append(wrapped, strings.TrimRight(rawLine[:splitAt], " "))
			rawLine = strings.TrimLeft(rawLine[splitAt:], " ")
		}

		wrapped = append(wrapped, rawLine)
	}

	return wrapped
}

func bestWrapIndex(value string, width int) int {
	runes := []rune(value)
	if len(runes) <= width {
		return len(value)
	}

	bestSpace := -1
	for index := 0; index < width && index < len(runes); index++ {
		if runes[index] == ' ' {
			bestSpace = index
		}
	}

	if bestSpace >= 0 {
		return len(string(runes[:bestSpace+1]))
	}

	return len(string(runes[:width]))
}

func formatTableLines(values []string, widths []int) []string {
	cellLines := make([][]string, len(widths))
	maxLines := 1
	for index := range widths {
		value := ""
		if index < len(values) {
			value = values[index]
		}
		lines := wrapLine(value, widths[index])
		if len(lines) == 0 {
			lines = []string{""}
		}
		cellLines[index] = lines
		maxLines = max(maxLines, len(lines))
	}

	lines := make([]string, 0, maxLines)
	for lineIndex := 0; lineIndex < maxLines; lineIndex++ {
		cells := make([]string, len(widths))
		for index := range widths {
			value := ""
			if lineIndex < len(cellLines[index]) {
				value = cellLines[index][lineIndex]
			}
			cells[index] = padRight(value, widths[index])
		}
		lines = append(lines, "  "+strings.Join(cells, "  "))
	}

	return lines
}

func formatTableDivider(widths []int) string {
	parts := make([]string, len(widths))
	for index, width := range widths {
		parts[index] = strings.Repeat("-", width)
	}

	return "  " + strings.Join(parts, "  ")
}

func (t Table) resolveColumnWidths() []int {
	widths := make([]int, len(t.Columns))
	minWidths := make([]int, len(t.Columns))
	totalNatural := 0

	for index, column := range t.Columns {
		widths[index] = runeWidth(column)
		minWidths[index] = max(4, min(widths[index], 16))
	}

	for _, row := range t.Rows {
		for index, value := range row {
			if index >= len(widths) {
				continue
			}
			natural := longestLineWidth(value)
			widths[index] = max(widths[index], natural)
			minWidths[index] = max(minWidths[index], min(max(8, longestWordWidth(value)), 20))
		}
	}

	for _, width := range widths {
		totalNatural += width
	}

	available := t.availableTableWidth(len(widths))
	if available <= 0 || totalNatural <= available {
		return widths
	}

	for totalNatural > available {
		column := widestShrinkableColumn(widths, minWidths)
		if column < 0 {
			break
		}
		widths[column]--
		totalNatural--
	}

	return widths
}

func (t Table) availableTableWidth(columnCount int) int {
	width := t.MaxWidth
	if width <= 0 {
		width = t.Features.Width
	}
	if width <= 0 {
		width = defaultTableWidth
	}

	available := width - 2 - max(0, (columnCount-1)*2)
	if available < columnCount*4 {
		return columnCount * 4
	}
	return available
}

func widestShrinkableColumn(widths, minWidths []int) int {
	bestIndex := -1
	bestWidth := 0
	for index, width := range widths {
		if width <= minWidths[index] {
			continue
		}
		if width > bestWidth {
			bestWidth = width
			bestIndex = index
		}
	}
	return bestIndex
}

func padRight(value string, width int) string {
	padding := width - runeWidth(value)
	if padding <= 0 {
		return value
	}

	return value + strings.Repeat(" ", padding)
}

func runeWidth(value string) int {
	return utf8.RuneCountInString(stripANSI(value))
}

func longestLineWidth(value string) int {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	width := 0
	for _, line := range lines {
		width = max(width, runeWidth(line))
	}
	return width
}

func longestWordWidth(value string) int {
	words := strings.Fields(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(stripANSI(value)))
	width := 0
	for _, word := range words {
		width = max(width, runeWidth(word))
	}
	return width
}

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func max(left, right int) int {
	if left > right {
		return left
	}

	return right
}

func min(left, right int) int {
	if left < right {
		return left
	}

	return right
}
