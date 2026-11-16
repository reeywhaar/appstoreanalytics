package asc

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"appstoreanalytics/internal/store"
)

// Measure is one number of one line of an analytics report.
type Measure struct {
	Day       string
	Metric    string
	Territory string
	Source    string
	// Item is Apple's identifier of what was bought, for purchases and proceeds, and "" for
	// every other measure. ItemName is what Apple calls it, and InApp says it was bought inside
	// an app rather than being the app.
	Item     string
	ItemName string
	InApp    bool
	Value    int64
}

// Reading says how one analytics report becomes measures.
type Reading struct {
	// Yields is every measure the report gives, all of which a newer instance of the same days
	// replaces.
	Yields []string
	// measure is what one line counts, and nothing for a line that is none of the kept ones.
	measure func(r row) []counted
}

type counted struct {
	metric string
	value  int64
}

func count(metric, column string) func(row) []counted {
	return func(r row) []counted { return []counted{{metric, r.integer(column)}} }
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
		Yields: store.SameReport(store.MetricImpressions),
		measure: func(r row) []counted {
			switch strings.ToLower(r.get("Event")) {
			case "impression":
				return []counted{{store.MetricImpressions, r.integer("Counts")}}
			case "page view":
				return []counted{{store.MetricPageViews, r.integer("Counts")}}
			}
			return nil
		},
	},
	"app downloads standard": {
		Yields: store.SameReport(store.MetricFirstDownloads),
		measure: func(r row) []counted {
			switch strings.ToLower(r.get("Download Type")) {
			case "first-time download":
				return []counted{{store.MetricFirstDownloads, r.integer("Counts")}}
			case "redownload":
				return []counted{{store.MetricRedownloads, r.integer("Counts")}}
			case "manual update", "auto-update":
				return []counted{{store.MetricUpdates, r.integer("Counts")}}
			}
			return nil
		},
	},
	"app store purchases standard": {
		Yields: store.SameReport(store.MetricPurchases),
		measure: func(r row) []counted {
			out := []counted{{store.MetricProceeds, r.micros("Proceeds in USD")}}
			if inApp(r) {
				out = append(out, counted{store.MetricPurchases, r.integer("Purchases")})
			}
			return out
		},
	},
	"app store installation and deletion standard": {
		Yields: store.SameReport(store.MetricInstalls),
		measure: func(r row) []counted {
			switch strings.ToLower(r.get("Event")) {
			case "install":
				return []counted{{store.MetricInstalls, r.integer("Counts")}}
			case "delete":
				return []counted{{store.MetricDeletions, r.integer("Counts")}}
			}
			return nil
		},
	},
	"app sessions standard": {
		Yields:  store.SameReport(store.MetricSessions),
		measure: count(store.MetricSessions, "Sessions"),
	},
	"app crashes": {
		Yields:  store.SameReport(store.MetricCrashes),
		measure: count(store.MetricCrashes, "Crashes"),
	},
}

// inApp is a purchase line of something bought inside an app — "In-app purchases" — rather than
// of the app itself, "App purchase".
func inApp(r row) bool {
	return strings.HasPrefix(strings.ToLower(r.get("Purchase Type")), "in-app")
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
		for _, c := range reading.measure(line) {
			if c.value == 0 {
				continue
			}
			out = append(out, Measure{
				Day:       line.get("Date"),
				Metric:    c.metric,
				Territory: line.get("Territory"),
				Source:    line.get("Source Type"),
				Item:      line.get("Content Apple Identifier"),
				ItemName:  line.get("Content Name"),
				InApp:     inApp(line),
				Value:     c.value,
			})
		}
	}
}
