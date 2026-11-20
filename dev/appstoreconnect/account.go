package main

import (
	"crypto/sha256"
	"math"
	"math/rand/v2"
	"strconv"
	"time"
)

// The invented account the demo reads: six apps of one small studio, what they sell, and how
// each one's days go. Every number is made from the app and the day alone, so a day reads the
// same whenever it is asked for and a run finds what an earlier one read unchanged.

// history is the first day of analytics reports: Apple's go back to 2024.
var history = date(2024, 1, 1)

// App is an app of the account and the shape of its days.
type App struct {
	ID, Name, Bundle, SKU string
	// Launched is its first day; an app older than the reports has history's.
	Launched time.Time
	// Downloads is first-time downloads on a weekday of its first year, before its season.
	Downloads float64
	// Growth is what a year multiplies its downloads, sessions and renewals by.
	Growth float64
	// Weekend is a Saturday's and a Sunday's downloads and sessions against a weekday's.
	Weekend float64
	// Season is a day's downloads against an ordinary one's, by the time of year.
	Season func(day time.Time) float64
	// Impressions and Views are per first-time download, on a day it is not featured.
	Impressions, Views float64
	// Redownloads is per first-time download.
	Redownloads float64
	// Updates is a day's updates in its first year, away from a release.
	Updates float64
	// Sessions is a day's sessions in its first year, of those who share analytics.
	Sessions float64
	// Crashes is per thousand sessions.
	Crashes float64
	// Release is how many days apart its versions come, and Featured how many days apart the
	// App Store features it; Phase puts both on their own days.
	Release, Featured, Phase int
	// Lean favors some countries over the account's usual spread.
	Lean  map[string]float64
	Items []Item
}

// Item is what an app sells: in-app purchases, or the app itself for a paid app.
type Item struct {
	ID, Name string
	// Proceeds is what one earns in the United States, in dollars.
	Proceeds float64
	// Rate is how many of a day's first-time downloads buy it that day.
	Rate float64
	// Renews is how many buy it again on a day of its first year: subscriptions renewing.
	Renews float64
	// Yearly renews a year after it was bought, rather than at a steady rate.
	Yearly bool
	// App is the app itself, bought: a paid app's download.
	App bool
}

// apps is the account. Ids are ten digits, as Apple's are.
var apps = []*App{
	{
		ID: "6447303215", Name: "Knotwork", Bundle: "com.example.knotwork", SKU: "KNOTWORK",
		Launched: history, Downloads: 520, Growth: 1.45, Weekend: 1.3,
		Season:      func(d time.Time) float64 { return 1 + 0.1*summer(d) },
		Impressions: 22, Views: 3, Redownloads: 0.16, Updates: 1700, Sessions: 6800, Crashes: 0.6,
		Release: 21, Featured: 143, Phase: 5,
		Lean: map[string]float64{"JP": 1.6, "KR": 1.6, "CN": 1.4, "TW": 1.4},
		Items: []Item{
			{ID: "6447304120", Name: "Handful of Hints", Proceeds: 0.84, Rate: 0.05},
			{ID: "6447304121", Name: "Chest of Coins", Proceeds: 4.24, Rate: 0.018},
			{ID: "6447304122", Name: "No More Ads", Proceeds: 2.54, Rate: 0.012},
		},
	},
	{
		ID: "1658824790", Name: "Tidepool", Bundle: "com.example.tidepool", SKU: "TIDEPOOL",
		Launched: history, Downloads: 210, Growth: 1.3, Weekend: 0.82,
		Season: func(d time.Time) float64 {
			return 1 + 1.6*math.Exp(-float64(d.YearDay())/12) + 0.25*bump(d, 245, 10)
		},
		Impressions: 30, Views: 3.4, Redownloads: 0.12, Updates: 900, Sessions: 5200, Crashes: 0.3,
		Release: 28, Featured: 197, Phase: 11,
		Lean: map[string]float64{"US": 1.15, "GB": 1.2, "CA": 1.1},
		Items: []Item{
			{ID: "1658825102", Name: "Tidepool Plus, monthly", Proceeds: 4.24, Rate: 0.012, Renews: 14},
			{ID: "1658825103", Name: "Tidepool Plus, yearly", Proceeds: 29.74, Rate: 0.018, Renews: 3.5, Yearly: true},
			{ID: "1658825104", Name: "Tidepool for life", Proceeds: 67.99, Rate: 0.002},
		},
	},
	{
		ID: "1590137452", Name: "Snowline", Bundle: "com.example.snowline", SKU: "SNOWLINE",
		Launched: history, Downloads: 210, Growth: 1.25, Weekend: 1.35,
		Season: func(d time.Time) float64 {
			c := math.Cos(2 * math.Pi * float64(d.YearDay()-20) / 365)
			return 0.08 + 1.1*math.Pow(math.Max(0, c), 1.4)
		},
		Impressions: 18, Views: 2.6, Redownloads: 0.3, Updates: 520, Sessions: 3000, Crashes: 0.5,
		Release: 35, Featured: 211, Phase: 17,
		Lean: map[string]float64{
			"CA": 3, "CH": 5, "AT": 5, "NO": 4, "SE": 2.5, "FI": 2.5, "FR": 1.6, "JP": 1.6, "NZ": 2,
			"BR": 0.1, "MX": 0.1, "IN": 0.1,
		},
		Items: []Item{
			{ID: "1590138001", Name: "Powder Pass, monthly", Proceeds: 2.54, Rate: 0.02, Renews: 9},
			{ID: "1590138002", Name: "Powder Pass, season", Proceeds: 12.74, Rate: 0.012},
		},
	},
	{
		ID: "1534409871", Name: "Beatkeeper", Bundle: "com.example.beatkeeper", SKU: "BEATKEEPER",
		Launched: history, Downloads: 95, Growth: 1.15, Weekend: 1.05,
		Season: func(d time.Time) float64 {
			return 1 + 0.35*bump(d, 250, 20) + 0.2*math.Exp(-float64(d.YearDay())/15)
		},
		Impressions: 25, Views: 3.2, Redownloads: 0.2, Updates: 400, Sessions: 1400, Crashes: 0.4,
		Release: 45, Featured: 167, Phase: 23,
		Lean: map[string]float64{"DE": 1.5, "JP": 1.5, "KR": 1.3, "AT": 1.3},
		Items: []Item{
			{ID: "1534410230", Name: "Beatkeeper Pro", Proceeds: 8.49, Rate: 0.035},
			{ID: "1534410231", Name: "Tip: a coffee", Proceeds: 2.54, Rate: 0.004},
		},
	},
	{
		ID: "1489932057", Name: "Herbarium", Bundle: "com.example.herbarium", SKU: "HERBARIUM",
		Launched: history, Downloads: 30, Growth: 1.05, Weekend: 1.25,
		Season:      func(d time.Time) float64 { return 1 + 0.55*summer(d) },
		Impressions: 60, Views: 6, Redownloads: 0.25, Updates: 210, Sessions: 380, Crashes: 0.3,
		Release: 60, Featured: 181, Phase: 3,
		Lean: map[string]float64{"GB": 2, "DE": 1.6, "NL": 1.6, "IE": 1.6, "CN": 0.2, "KR": 0.4},
		Items: []Item{
			{ID: "1489932057", Name: "Herbarium", Proceeds: 3.39, Rate: 1, App: true},
			{ID: "1489933410", Name: "Mosses and Lichens", Proceeds: 1.69, Rate: 0.04},
		},
	},
	{
		ID: "6502116839", Name: "Fieldnote", Bundle: "com.example.fieldnote", SKU: "FIELDNOTE",
		Launched: date(2025, 6, 12), Downloads: 60, Growth: 1.6, Weekend: 0.9,
		Season: func(d time.Time) float64 {
			return 1 + 0.6*math.Exp(-float64(d.YearDay())/14)
		},
		Impressions: 35, Views: 3.6, Redownloads: 0.1, Updates: 250, Sessions: 900, Crashes: 0.5,
		Release: 24, Featured: 229, Phase: 9,
		Lean: map[string]float64{"US": 1.25, "GB": 1.25, "AU": 1.2},
		Items: []Item{
			{ID: "6502117304", Name: "Fieldnote Plus, yearly", Proceeds: 21.24, Rate: 0.03, Yearly: true},
			{ID: "6502117305", Name: "Fieldnote for life", Proceeds: 50.99, Rate: 0.006},
		},
	},
}

// appByID finds an app of the account.
func appByID(id string) *App {
	for _, a := range apps {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// Figures is what an app did on a day, before it is split by country and source.
type Figures struct {
	Downloads, Redownloads, Updates        float64
	Impressions, Views                     float64
	Installs, Deletions, Sessions, Crashes float64
	// Featured is how much the App Store is featuring the app, from 0 to 1, which sends
	// visitors from browsing rather than searching.
	Featured float64
	// Bought is the purchases each item is expected to make, by its index.
	Bought []float64
	// Version is the version out that day, and Previous the one before it.
	Version, Previous string
}

// figures is the app's day: nothing before it was launched.
func (a *App) figures(day time.Time) Figures {
	if day.Before(a.Launched) {
		return Figures{}
	}
	r := rng(a.ID, day, "figures")
	noise := func(spread float64) float64 { return math.Exp(spread * r.NormFloat64()) }
	years := day.Sub(a.Launched).Hours() / 24 / 365
	grown := math.Pow(a.Growth, years)
	week := 1.0
	if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
		week = a.Weekend
	}
	featured := a.featured(day)
	base := a.level(day)

	f := Figures{Featured: featured}
	f.Downloads = base * (1 + 2.5*featured) * a.launch(day)
	f.Redownloads = base * a.Redownloads * noise(0.15)
	f.Impressions = base * a.Impressions * (1 + 5*featured) * a.launch(day) * noise(0.1)
	f.Views = base * a.Views * (1 + 3*featured) * a.launch(day) * noise(0.1)
	f.Installs = (f.Downloads + f.Redownloads) * 0.45 * noise(0.08)
	f.Deletions = f.Installs*0.3*noise(0.15) + a.Sessions*grown*0.004*noise(0.2)

	since, n := a.release(day)
	f.Updates = a.Updates * grown * (1 + 3.5*math.Exp(-float64(since)/1.8)) * noise(0.1)
	f.Sessions = a.Sessions * grown * week * math.Pow(a.Season(day), 0.8) * a.wobble(day) * noise(0.07)
	f.Crashes = f.Sessions * a.Crashes / 1000 * noise(0.3)
	// Every fifth version crashes more for its first three days, until its fix is out.
	if n%5 == 4 && since < 3 {
		f.Crashes *= 6
	}
	f.Version, f.Previous = version(n), version(n-1)

	f.Bought = make([]float64, len(a.Items))
	for i, it := range a.Items {
		f.Bought[i] = a.bought(it, day, f.Downloads, grown)
	}
	return f
}

// level is the app's first-time downloads on a day, unfeatured: its size and growth, the week,
// the season and the holidays, and a day's chance.
func (a *App) level(day time.Time) float64 {
	if day.Before(a.Launched) {
		return 0
	}
	years := day.Sub(a.Launched).Hours() / 24 / 365
	week := 1.0
	if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
		week = a.Weekend
	}
	r := rng(a.ID, day, "level")
	return a.Downloads * math.Pow(a.Growth, years) * week * a.Season(day) * holidays(day) *
		a.wobble(day) * math.Exp(0.12*r.NormFloat64())
}

// bought is how many of an item a day is expected to sell: to that day's downloads, and to
// subscribers renewing.
func (a *App) bought(it Item, day time.Time, downloads, grown float64) float64 {
	n := it.Rate * downloads
	switch {
	case it.Yearly:
		// A year's subscribers renew on the day they bought, most of them; before there are any,
		// the subscribers the app had before the reports.
		ago := day.AddDate(-1, 0, 0)
		if ago.Before(a.Launched) {
			n += it.Renews * grown
		} else {
			n += 0.62 * it.Rate * a.level(ago) * (1 + 2.5*a.featured(ago)) * a.launch(ago)
			n += 0.5 * it.Renews * grown
		}
	case it.Renews > 0:
		n += it.Renews * grown * math.Sqrt(a.Season(day))
	}
	return n
}

// featured is how much the App Store features the app on a day: for a week every so often,
// most at first.
func (a *App) featured(day time.Time) float64 {
	d := int(day.Sub(history).Hours()/24) + a.Phase*7
	k := d % a.Featured
	if k >= 10 {
		return 0
	}
	return math.Exp(-float64(k) / 3)
}

// launch is the lift of an app's first weeks, for an app newer than the reports.
func (a *App) launch(day time.Time) float64 {
	if !a.Launched.After(history) {
		return 1
	}
	d := day.Sub(a.Launched).Hours() / 24
	return 1 + 7*math.Exp(-d/6)
}

// release is how many days ago the app's latest version came out, and which it is.
func (a *App) release(day time.Time) (since, n int) {
	d := int(day.Sub(history).Hours()/24) + a.Phase
	return d % a.Release, d / a.Release
}

// wobble is a few weeks up and a few down, as an app has for reasons of its own.
func (a *App) wobble(day time.Time) float64 {
	t := day.Sub(history).Hours() / 24
	p := float64(a.Phase)
	return 1 + 0.08*math.Sin(2*math.Pi*t/23+p) + 0.06*math.Sin(2*math.Pi*t/61+2*p)
}

func version(n int) string {
	if n < 0 {
		n = 0
	}
	return strconv.Itoa(2+n/12) + "." + strconv.Itoa(n%12)
}

// holidays is new phones and tablets at Christmas, and time to try apps on them.
func holidays(day time.Time) float64 {
	m, d := day.Month(), day.Day()
	switch {
	case m == time.December && d == 25:
		return 2.4
	case m == time.December && d == 26:
		return 1.9
	case m == time.December && d >= 27:
		return 1.5
	case m == time.January && d <= 2:
		return 1.3
	}
	return 1
}

// summer rises from spring to the end of June and falls by autumn, from 0 to 1.
func summer(day time.Time) float64 {
	return math.Max(0, math.Sin(2*math.Pi*float64(day.YearDay()-80)/365))
}

// bump is a rise of the given width, in days, around a day of the year, from 0 to 1.
func bump(day time.Time, around, width int) float64 {
	d := float64(day.YearDay() - around)
	return math.Exp(-d * d / float64(2*width*width))
}

// Where an app's downloads come from: countries by their share of the account's usual spread,
// and what a proceeds line earns there against the United States.
var territories = []struct {
	Code          string
	Share, Priced float64
}{
	{"US", 0.31, 1}, {"GB", 0.07, 1.02}, {"DE", 0.065, 0.97}, {"JP", 0.06, 0.86},
	{"FR", 0.045, 0.97}, {"CA", 0.045, 0.92}, {"AU", 0.035, 0.9}, {"CN", 0.035, 0.86},
	{"KR", 0.025, 0.9}, {"IT", 0.025, 0.97}, {"ES", 0.025, 0.97}, {"BR", 0.02, 0.58},
	{"NL", 0.018, 0.97}, {"IN", 0.018, 0.42}, {"MX", 0.015, 0.62}, {"SE", 0.012, 0.95},
	{"CH", 0.012, 1.05}, {"PL", 0.012, 0.8}, {"TW", 0.01, 0.9}, {"AT", 0.008, 0.97},
	{"NO", 0.008, 0.98}, {"IE", 0.006, 0.97}, {"NZ", 0.006, 0.9}, {"FI", 0.005, 0.97},
}

// spread is an app's share of each country, in the order of territories, summing to one.
func (a *App) spread() []float64 {
	out := make([]float64, len(territories))
	var sum float64
	for i, t := range territories {
		w := t.Share
		if lean, ok := a.Lean[t.Code]; ok {
			w *= lean
		}
		out[i] = w
		sum += w
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// sources is where a visitor came from, by the name Apple gives it, and its share on a day the
// app is not featured.
var sources = []struct {
	Name  string
	Share float64
}{
	{"App Store search", 0.52}, {"App Store browse", 0.18}, {"Web referrer", 0.09},
	{"App referrer", 0.08}, {"Unavailable", 0.13},
}

// sourceShares is each source's share on a day featured that much: browsing brings more.
func sourceShares(featured float64) []float64 {
	out := make([]float64, len(sources))
	var sum float64
	for i, s := range sources {
		w := s.Share
		if s.Name == "App Store browse" {
			w *= 1 + 8*featured
		}
		out[i] = w
		sum += w
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// rng is the chance of one app's day, for one purpose: the same every time it is asked.
func rng(app string, day time.Time, purpose string) *rand.Rand {
	return rand.New(rand.NewChaCha8(sha256.Sum256([]byte(app + "|" + day.Format(time.DateOnly) + "|" + purpose))))
}

// poisson is how many of something with that many expected happened.
func poisson(r *rand.Rand, mean float64) int64 {
	if mean <= 0 {
		return 0
	}
	if mean < 30 {
		limit, k, p := math.Exp(-mean), int64(0), 1.0
		for {
			p *= r.Float64()
			if p <= limit {
				return k
			}
			k++
		}
	}
	return max(0, int64(math.Round(mean+math.Sqrt(mean)*r.NormFloat64())))
}

// pick is an index drawn by the shares given.
func pick(r *rand.Rand, shares []float64) int {
	x := r.Float64()
	for i, s := range shares {
		if x < s {
			return i
		}
		x -= s
	}
	return len(shares) - 1
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
