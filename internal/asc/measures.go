package asc

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"appstoreanalytics/internal/store"
)

// Measure is one line of an analytics report, as one of the measures kept.
type Measure struct {
	Day       string
	Metric    string
	Territory string
	Source    string
	Value     int64
}

// Reading says how one analytics report becomes measures.
type Reading struct {
	// Yields is every measure the report gives, all of which a newer instance of the same days
	// replaces.
	Yields []string
	// measure is the measure a line is and the column holding its number, and "" for a line
	// that is none of the kept ones.
	measure func(r row) (metric, column string)
}

// ReadingOf is how a report is read, and false for a report that is not. Apple's names are
// compared without regard to case, as it says its values should be.
func ReadingOf(name string) (Reading, bool) {
	r, ok := readings[strings.ToLower(name)]
	return r, ok
}

// readings are the reports read, by the name Apple gives them, lowercased. Only the Standard
// ones: the Detailed ones say the same split finer, and keep fewer lines under Apple's privacy
// threshold. See docs/app-store-connect.md.
var readings = map[string]Reading{
	"app store discovery and engagement standard": {
		Yields: []string{store.MetricImpressions, store.MetricPageViews},
		measure: func(r row) (string, string) {
			switch strings.ToLower(r.get("Event")) {
			case "impression":
				return store.MetricImpressions, "Counts"
			case "page view":
				return store.MetricPageViews, "Counts"
			}
			return "", ""
		},
	},
	"app downloads standard": {
		Yields: []string{store.MetricFirstDownloads, store.MetricRedownloads},
		measure: func(r row) (string, string) {
			switch strings.ToLower(r.get("Download Type")) {
			case "first-time download":
				return store.MetricFirstDownloads, "Counts"
			case "redownload":
				return store.MetricRedownloads, "Counts"
			}
			return "", ""
		},
	},
	"app store installation and deletion standard": {
		Yields: []string{store.MetricInstalls, store.MetricDeletions},
		measure: func(r row) (string, string) {
			switch strings.ToLower(r.get("Event")) {
			case "install":
				return store.MetricInstalls, "Counts"
			case "delete":
				return store.MetricDeletions, "Counts"
			}
			return "", ""
		},
	},
	"app sessions standard": {
		Yields:  []string{store.MetricSessions},
		measure: func(row) (string, string) { return store.MetricSessions, "Sessions" },
	},
	"app crashes": {
		Yields:  []string{store.MetricCrashes},
		measure: func(row) (string, string) { return store.MetricCrashes, "Crashes" },
	},
}

// ParseReport reads one file of an analytics report.
func ParseReport(name string, r io.Reader) ([]Measure, error) {
	reading, ok := ReadingOf(name)
	if !ok {
		return nil, fmt.Errorf("%q is not a report this reads", name)
	}
	t, err := newTable(r)
	if err != nil {
		return nil, err
	}
	if !t.has("Date") {
		return nil, fmt.Errorf("the %s report has no Date column, so it is not one this reads", name)
	}
	var out []Measure
	for {
		line, err := t.next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		metric, column := reading.measure(line)
		if metric == "" {
			continue
		}
		value := line.integer(column)
		if value == 0 {
			continue
		}
		out = append(out, Measure{
			Day:       line.get("Date"),
			Metric:    metric,
			Territory: line.get("Territory"),
			Source:    line.get("Source Type"),
			Value:     value,
		})
	}
}
