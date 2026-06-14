package format

import (
	"encoding/csv"
	"strings"
)

// CSVFormatter renders CSV/TSV lines as an HTML table. The first line is
// treated as the header row. Tab-separated values are detected automatically.
type CSVFormatter struct{}

func (f *CSVFormatter) Name() string { return NameCSV }

func (f *CSVFormatter) Format(lines []string) (string, error) {
	if len(lines) == 0 {
		return "<div class=\"rdw-csv\"></div>", nil
	}

	// Detect delimiter: if the first line has more tabs than commas, use TSV.
	delim := ','
	if strings.Count(lines[0], "\t") > strings.Count(lines[0], ",") {
		delim = '\t'
	}

	var rows [][]string

	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}

		r := csv.NewReader(strings.NewReader(l))
		r.Comma = rune(delim)
		r.FieldsPerRecord = -1

		fields, err := r.Read()
		if err != nil {
			// Malformed line — emit as single-cell row.
			rows = append(rows, []string{l})
			continue
		}

		rows = append(rows, fields)
	}

	if len(rows) == 0 {
		return "<div class=\"rdw-csv\"></div>", nil
	}

	var b strings.Builder

	b.WriteString("<div class=\"rdw-csv\"><table class=\"rdw-csv-table\">")

	// Header row.
	b.WriteString("<thead><tr>")
	for i, cell := range rows[0] {
		b.WriteString("<th data-col=\"")
		b.WriteString(escapeHTML(string(rune('0'+i%10))))
		b.WriteString("\">")
		b.WriteString(escapeHTML(cell))
		b.WriteString("</th>")
	}
	b.WriteString("</tr></thead>")

	// Data rows.
	if len(rows) > 1 {
		b.WriteString("<tbody>")
		for _, row := range rows[1:] {
			b.WriteString("<tr>")
			for _, cell := range row {
				b.WriteString("<td>")
				b.WriteString(escapeHTML(cell))
				b.WriteString("</td>")
			}
			b.WriteString("</tr>")
		}
		b.WriteString("</tbody>")
	}

	b.WriteString("</table></div>")

	return b.String(), nil
}
