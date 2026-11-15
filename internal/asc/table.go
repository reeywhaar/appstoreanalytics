package asc

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// table reads a report: gzipped or not, tab-separated or comma-separated, with a header row.
// Columns are found by name, so a report version that adds one or moves one reads the same.
//
// A tab-separated report is split on tabs and nothing else: Apple does not quote in one, and a
// title may hold a quote mark that a CSV reader would take for the start of a quoted field.
type table struct {
	lines   *bufio.Reader
	quoted  *csv.Reader
	columns map[string]int
}

func newTable(r io.Reader) (*table, error) {
	br := bufio.NewReader(r)
	if magic, _ := br.Peek(2); bytes.Equal(magic, []byte{0x1f, 0x8b}) {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("unzip the report: %w", err)
		}
		br = bufio.NewReader(gz)
	}
	first, err := br.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read the report's header: %w", err)
	}
	first = strings.TrimPrefix(first, "\uFEFF")
	sep := '\t'
	if !strings.Contains(first, "\t") {
		sep = ','
	}
	t := &table{columns: map[string]int{}}
	var header []string
	if sep == '\t' {
		t.lines = br
		header = strings.Split(strings.TrimRight(first, "\r\n"), "\t")
	} else {
		r := csv.NewReader(strings.NewReader(first))
		if header, err = r.Read(); err != nil {
			return nil, fmt.Errorf("read the report's header: %w", err)
		}
		t.quoted = csv.NewReader(br)
		t.quoted.FieldsPerRecord = -1
		t.quoted.LazyQuotes = true
	}
	for i, name := range header {
		t.columns[strings.ToLower(strings.Trim(strings.TrimSpace(name), `"`))] = i
	}
	return t, nil
}

// record is the next line's fields.
func (t *table) record() ([]string, error) {
	if t.quoted != nil {
		return t.quoted.Read()
	}
	line, err := t.lines.ReadString('\n')
	if line == "" && err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(line, "\r\n"), "\t"), nil
}

// has reports whether the report has a column.
func (t *table) has(name string) bool {
	_, ok := t.columns[strings.ToLower(name)]
	return ok
}

// next is the following row, and io.EOF after the last.
func (t *table) next() (row, error) {
	for {
		rec, err := t.record()
		if err != nil {
			return row{}, err
		}
		// A sales report ends in a blank line or two.
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		return row{t: t, rec: rec}, nil
	}
}

type row struct {
	t   *table
	rec []string
}

// get is a column's value, and "" when the report has no such column.
func (r row) get(name string) string {
	i, ok := r.t.columns[strings.ToLower(name)]
	if !ok || i >= len(r.rec) {
		return ""
	}
	return strings.TrimSpace(r.rec[i])
}

// integer reads a whole number, which a report may write with a decimal point.
func (r row) integer(name string) int64 {
	v := r.get(name)
	if v == "" {
		return 0
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f))
}

// micros reads an amount of money as millionths, so that sums of them are exact.
func (r row) micros(name string) int64 {
	f, err := strconv.ParseFloat(strings.ReplaceAll(r.get(name), ",", ""), 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f * 1e6))
}
