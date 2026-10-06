// CSV rendering: format CSV files as tables, using the exact same
// rendering pipeline as tables inside markdown files.
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

var csvExtensions = map[string]bool{
	".csv": true,
}

func isCSV(name string) bool {
	return csvExtensions[strings.ToLower(filepath.Ext(name))]
}

// renderCSV formats CSV content as a table by converting it to a
// markdown table and rendering it with the same markdown renderer used
// for .md files, so the output is identical to a markdown table: same
// glamour styling, same dark/ascii behavior, same CATT_COLOR handling.
//
// The first CSV row becomes the table header. Cells are padded/truncated
// to the widest row so ragged CSVs still produce a well-formed table.
// Pipe characters are escaped, and newlines inside quoted fields are
// collapsed to spaces (markdown table cells cannot span lines).
func renderCSV(content string, w io.Writer) error {
	records, err := parseCSV(content)
	if err != nil {
		return fmt.Errorf("failed to parse CSV: %w", err)
	}
	if len(records) == 0 {
		return renderMarkdown(content, w)
	}

	width := 0
	for _, row := range records {
		if len(row) > width {
			width = len(row)
		}
	}

	var b strings.Builder
	for i, row := range records {
		if i == 1 {
			// Markdown header separator after the first row.
			b.WriteString("|" + strings.Repeat(" --- |", width) + "\n")
		}
		b.WriteString("|")
		for j := 0; j < width; j++ {
			cell := ""
			if j < len(row) {
				cell = row[j]
			}
			b.WriteString(" " + escapeMarkdownCell(cell) + " |")
		}
		b.WriteString("\n")
	}

	return renderMarkdown(b.String(), w)
}

// parseCSV reads all records from CSV content.
func parseCSV(content string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(content))
	r.FieldsPerRecord = -1 // tolerate ragged rows
	return r.ReadAll()
}

// escapeMarkdownCell makes a CSV cell safe inside a markdown table:
// escape pipes, collapse newlines to spaces.
func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}
