// CSV rendering: format CSV files as tables with the exact same
// appearance as tables inside markdown files.
//
// Instead of converting the CSV to a markdown table and feeding it
// through glamour's markdown parser, the table is built directly with
// lipgloss/table, mirroring what glamour's ansi.TableElement does
// internally (glamour/ansi/table.go). This keeps the styling identical
// to markdown tables while treating cell contents as raw CSV data:
// markdown syntax in cells (*emphasis*, `code`, [links], :emoji: …) is
// displayed verbatim instead of being interpreted.
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/termenv"
)

var csvExtensions = map[string]bool{
	".csv": true,
}

func isCSV(name string) bool {
	return csvExtensions[strings.ToLower(filepath.Ext(name))]
}

// csvWrapWidth mirrors glamour's default WordWrap width (glamour.go:
// defaultWidth). catt does not set WithWordWrap, so glamour renders
// markdown tables at this width; the CSV table uses the same value.
const csvWrapWidth = 80

// renderCSV formats CSV content as a table. The first CSV row becomes
// the table header. Cells are padded/truncated to the widest row so
// ragged CSVs still produce a well-formed table. Newlines inside quoted
// fields are collapsed to spaces so cells stay single-line, as in
// markdown tables. Everything else in a cell is shown verbatim.
func renderCSV(content string, w io.Writer) error {
	records, err := parseCSV(content)
	if err != nil {
		return fmt.Errorf("failed to parse CSV: %w", err)
	}
	if len(records) == 0 {
		// No records (e.g. empty file): fall back to the markdown
		// renderer, same as an empty .md file.
		return renderMarkdown(content, w)
	}

	dark := useDarkStyle()
	// Match glamour, which renders with a fixed TrueColor profile
	// regardless of terminal detection.
	lipgloss.SetColorProfile(termenv.TrueColor)

	fmt.Fprint(w, stripTrailingPadding(renderCSVTable(records, dark)))
	return nil
}

// renderCSVTable renders records as a lipgloss table padded like a
// glamour document block: every line padded out to the wrap width
// (glamour's MarginWriter padding). Unlike a glamour markdown block,
// no leading blank line or trailing newline is emitted; the output
// starts at the table's first line and ends at its last.
func renderCSVTable(records [][]string, dark bool) string {
	width := 0
	for _, row := range records {
		if len(row) > width {
			width = len(row)
		}
	}

	// Dark style leaves table separators unset, so glamour falls back
	// to lipgloss.NormalBorder. The ASCII style configures row/column
	// separators of "-" and "|" (styles/styles.go: ASCIIStyleConfig).
	border := lipgloss.NormalBorder()
	if !dark {
		border = lipgloss.Border{
			Top:    "-",
			Bottom: "-",
			Left:   "|",
			Right:  "|",
			Middle: "|",
		}
	}

	cellStyle := lipgloss.NewStyle().Inline(false).Margin(0, 1)
	if dark {
		// The dark style's document color cascades into table cells
		// (glamour/ansi/baseelement.go: StyleOverrideRender).
		cellStyle = cellStyle.Foreground(lipgloss.Color("252"))
	}

	t := table.New().
		Width(csvWrapWidth).
		Wrap(true). // glamour defaults to wrapping table content
		StyleFunc(func(_, col int) lipgloss.Style {
			return cellStyle
		}).
		Border(border).
		BorderTop(false).
		BorderLeft(false).
		BorderRight(false).
		BorderBottom(false)

	headers := make([]string, width)
	for j, cell := range records[0] {
		headers[j] = sanitizeCSVCell(cell)
	}
	t = t.Headers(headers...)

	for _, row := range records[1:] {
		cells := make([]string, width)
		for j := 0; j < width && j < len(row); j++ {
			cells[j] = sanitizeCSVCell(row[j])
		}
		t = t.Row(cells...)
	}
	body := t.String()

	var b strings.Builder
	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(padCSVLine(line, dark))
	}
	return b.String()
}

// padCSVLine pads a line out to the wrap width with single spaces,
// replicating glamour's MarginWriter: the document style colors the
// padding spaces in the dark style; they are plain spaces otherwise.
func padCSVLine(line string, dark bool) string {
	var b strings.Builder
	b.WriteString(line)
	pad := lipgloss.NewStyle()
	if dark {
		pad = pad.Foreground(lipgloss.Color("252"))
	}
	for i := lipgloss.Width(line); i < csvWrapWidth; i++ {
		b.WriteString(pad.Render(" "))
	}
	return b.String()
}

// parseCSV reads all records from CSV content.
func parseCSV(content string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(content))
	r.FieldsPerRecord = -1 // tolerate ragged rows
	return r.ReadAll()
}

// sanitizeCSVCell makes a CSV cell safe to place in a single-line
// table: collapse newlines (markdown table cells cannot span lines).
// No markdown escaping is needed because cell contents are not parsed
// as markdown; pipes and other special characters are shown verbatim.
func sanitizeCSVCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}
