package main

import (
	"bufio"
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"time"
)

// Report is one of the reports a request makes, and how its lines for an app's day are written.
// Lines are split as finely as what the dashboard splits them by: downloads, impressions and
// page views by country and source, everything else by country.
type Report struct {
	Name   string
	Header string
	// Lines writes a day's lines; nil for a report the app does not read, listed only.
	Lines func(w io.Writer, a *App, day time.Time, f Figures, r *rand.Rand)
}

// private is the fewest devices Apple reports a count of in the reports of those who share
// analytics with developers: installs, deletions and sessions.
const private = 5

// reports are what every request makes, in Apple's names: the six read, and some of those that
// are not.
var reports = []Report{
	{
		Name:   "App Downloads Standard",
		Header: "Date\tApp Name\tApp Apple Identifier\tDownload Type\tApp Version\tDevice\tPlatform Version\tSource Type\tPage Type\tPre-Order\tTerritory\tCounts",
		Lines: func(w io.Writer, a *App, day time.Time, f Figures, r *rand.Rand) {
			line := func(kind, source, territory string, n int64) {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\tiPhone\tiOS 26.1\t%s\tProduct page\tNo\t%s\t%d\n",
					day.Format(time.DateOnly), a.Name, a.ID, kind, f.Version, source, territory, n)
			}
			spread, shares := a.spread(), sourceShares(f.Featured)
			eachCell(r, f.Downloads, spread, shares, func(s, t string, n int64) { line("First-time download", s, t, n) })
			eachCountry(r, f.Redownloads, spread, 1, func(t string, n int64) { line("Redownload", sources[pick(r, shares)].Name, t, n) })
			eachCountry(r, f.Updates, spread, 1, func(t string, n int64) {
				source := sources[pick(r, shares)].Name
				if manual := n / 6; manual > 0 {
					line("Manual update", source, t, manual)
					n -= manual
				}
				line("Auto-update", source, t, n)
			})
		},
	},
	{
		Name:   "App Store Discovery and Engagement Standard",
		Header: "Date\tApp Name\tApp Apple Identifier\tEvent\tPage Type\tSource Type\tEngagement Type\tDevice\tPlatform Version\tTerritory\tCounts\tUnique Counts",
		Lines: func(w io.Writer, a *App, day time.Time, f Figures, r *rand.Rand) {
			spread, shares := a.spread(), sourceShares(f.Featured)
			for _, e := range []struct {
				event string
				n     float64
			}{{"Impression", f.Impressions}, {"Page view", f.Views}} {
				eachCell(r, e.n, spread, shares, func(s, t string, n int64) {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\tProduct page\t%s\t\tiPhone\tiOS 26.1\t%s\t%d\t%d\n",
						day.Format(time.DateOnly), a.Name, a.ID, e.event, s, t, n, max(1, n*4/5))
				})
			}
		},
	},
	{
		Name:   "App Store Purchases Standard",
		Header: "Date\tApp Name\tApp Apple Identifier\tPurchase Type\tContent Name\tContent Apple Identifier\tPayment Method\tDevice\tPlatform Version\tSource Type\tPage Type\tApp Download Date\tPre-Order\tTerritory\tPurchases\tProceeds in USD\tSales in USD\tPaying Users",
		Lines: func(w io.Writer, a *App, day time.Time, f Figures, r *rand.Rand) {
			spread, shares := a.spread(), sourceShares(f.Featured)
			for i, it := range a.Items {
				kind := "In-app purchases"
				if it.App {
					kind = "App purchase"
				}
				line := func(t int, n int64) {
					proceeds := float64(n) * it.Proceeds * territories[t].Priced
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\tApple ID\tiPhone\tiOS 26.1\t%s\tProduct page\t%s\tNo\t%s\t%d\t%.2f\t%.2f\t%d\n",
						day.Format(time.DateOnly), a.Name, a.ID, kind, it.Name, it.ID, sources[pick(r, shares)].Name,
						day.Format(time.DateOnly), territories[t].Code, n, proceeds, proceeds/0.85, max(1, abs(n)))
				}
				for t, share := range spread {
					if n := poisson(r, f.Bought[i]*share); n > 0 {
						line(t, n)
					}
				}
				// Refunds, as purchases taken back.
				for range poisson(r, f.Bought[i]*0.012) {
					line(pick(r, spread), -1)
				}
			}
		},
	},
	{
		Name:   "App Store Installation and Deletion Standard",
		Header: "Date\tApp Name\tApp Apple Identifier\tEvent\tDownload Type\tApp Version\tDevice\tPlatform Version\tSource Type\tPage Type\tTerritory\tCounts\tUnique Devices",
		Lines: func(w io.Writer, a *App, day time.Time, f Figures, r *rand.Rand) {
			spread, shares := a.spread(), sourceShares(f.Featured)
			for _, e := range []struct {
				event string
				n     float64
			}{{"Install", f.Installs}, {"Delete", f.Deletions}} {
				eachCountry(r, e.n, spread, private, func(t string, n int64) {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\tFirst-time download\t%s\tiPhone\tiOS 26.1\t%s\tProduct page\t%s\t%d\t%d\n",
						day.Format(time.DateOnly), a.Name, a.ID, e.event, f.Version, sources[pick(r, shares)].Name, t, n, n)
				})
			}
		},
	},
	{
		Name:   "App Sessions Standard",
		Header: "Date\tApp Name\tApp Apple Identifier\tApp Version\tDevice\tPlatform Version\tSource Type\tPage Type\tApp Download Date\tTerritory\tSessions\tTotal Session Duration\tUnique Devices",
		Lines: func(w io.Writer, a *App, day time.Time, f Figures, r *rand.Rand) {
			spread, shares := a.spread(), sourceShares(f.Featured)
			eachCountry(r, f.Sessions, spread, private, func(t string, n int64) {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\tiPhone\tiOS 26.1\t%s\tProduct page\t\t%s\t%d\t%d\t%d\n",
					day.Format(time.DateOnly), a.Name, a.ID, f.Version, sources[pick(r, shares)].Name, t, n, n*190, max(1, n*2/5))
			})
		},
	},
	{
		Name:   "App Crashes",
		Header: "Date\tApp Name\tApp Apple Identifier\tApp Version\tBuild\tDevice\tPlatform Version\tCrashes\tUnique Devices",
		Lines: func(w io.Writer, a *App, day time.Time, f Figures, r *rand.Rand) {
			for _, v := range []struct {
				version string
				share   float64
			}{{f.Version, 0.8}, {f.Previous, 0.2}} {
				if n := poisson(r, f.Crashes*v.share); n > 0 {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t1\tiPhone\tiOS 26.1\t%d\t%d\n",
						day.Format(time.DateOnly), a.Name, a.ID, v.version, n, max(1, n*3/4))
				}
			}
		},
	},
	{Name: "App Store Discovery and Engagement Detailed"},
	{Name: "App Downloads Detailed"},
	{Name: "App Store Installation and Deletion Detailed"},
	{Name: "App Store Web Preview Engagement"},
}

// reportNamed finds a report by its name.
func reportNamed(name string) *Report {
	for i := range reports {
		if reports[i].Name == name {
			return &reports[i]
		}
	}
	return nil
}

// write writes a report's file of an app's days, from first to last.
func (rep *Report) write(w io.Writer, a *App, first, last time.Time) error {
	b := bufio.NewWriterSize(w, 64<<10)
	fmt.Fprintln(b, rep.Header)
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		if day.Before(a.Launched) {
			continue
		}
		rep.Lines(b, a, day, a.figures(day), rng(a.ID, day, rep.Name))
	}
	return b.Flush()
}

// crossed is how many of an app's largest countries a count is split among sources in. The
// rest are each given one source, drawn by the same shares, which comes to the same shares over
// days and keeps the lines few.
const crossed = 3

// eachCell splits a day's count among countries and sources, and says each part that is
// something.
func eachCell(r *rand.Rand, total float64, spread, shares []float64, each func(source, territory string, n int64)) {
	big := slices.Sorted(slices.Values(spread))[len(spread)-crossed]
	for t, ts := range spread {
		if ts < big {
			if n := poisson(r, total*ts); n > 0 {
				each(sources[pick(r, shares)].Name, territories[t].Code, n)
			}
			continue
		}
		for s, ss := range shares {
			if n := poisson(r, total*ts*ss); n > 0 {
				each(sources[s].Name, territories[t].Code, n)
			}
		}
	}
}

// eachCountry splits a day's count among countries, and says each part of at least least.
func eachCountry(r *rand.Rand, total float64, spread []float64, least int64, each func(territory string, n int64)) {
	for t, ts := range spread {
		if n := poisson(r, total*ts); n >= max(1, least) {
			each(territories[t].Code, n)
		}
	}
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
