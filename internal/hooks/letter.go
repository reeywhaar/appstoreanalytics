package hooks

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Fact is one thing a delivery says, labelled for a person.
type Fact struct {
	Label string
	Value string
}

// Title is an event as a person names it: "Test notification", "Subscribed: initial buy". One
// Apple has not documented is its own name, spaced out.
func Title(event string) string {
	kind, sub, _ := strings.Cut(event, "/")
	title := words(kind)
	if k, ok := kindOf(kind); ok {
		title = k.Title
	}
	if sub != "" {
		title += ": " + strings.ToLower(words(sub))
	}
	return title
}

// Meaning is what Apple says an event means, "" for one it does not document. A subtype means
// what its type does.
func Meaning(event string) string {
	kind, _, _ := strings.Cut(event, "/")
	k, _ := kindOf(kind)
	return k.Meaning
}

func kindOf(name string) (Kind, bool) {
	for _, service := range Catalog {
		for _, g := range service.Groups {
			for _, k := range g.Kinds {
				if k.Name == name {
					return k, true
				}
			}
		}
	}
	return Kind{}, false
}

// About is what mailing a delivery knows that its body does not.
type About struct {
	// App is the name of the app a notification is about, "" when it is not one of the apps.
	App string
	// Via is the label of the address it came through.
	Via string
	// ProvenBy is who it is proven to come from: ByApple, BySecret.
	ProvenBy string
	Received time.Time
}

// Letter is a delivery as a message to a person: a subject saying what happened and to which
// app, then the same in labelled lines, what Apple says the event means, and the body itself.
func Letter(d Delivery, body []byte, a About) (subject, text string) {
	title := Title(d.Event)
	headline := title
	// A state that moved says where to, which is what the subject is read for.
	for _, f := range d.Facts {
		if _, after, ok := strings.Cut(f.Value, " → "); ok {
			headline += ": " + after
			break
		}
	}
	about := a.App
	if about == "" {
		about = cmp.Or(d.BundleID, a.Via)
	}
	subject = join(headline, about)
	if d.Environment == "Sandbox" {
		subject += " (Sandbox)"
	}

	lines := []Fact{{"Event", title}}
	switch {
	case a.App != "" && d.BundleID != "":
		lines = append(lines, Fact{"App", a.App + " (" + d.BundleID + ")"})
	case d.BundleID != "":
		lines = append(lines, Fact{"App", d.BundleID})
	}
	if d.Environment != "" {
		lines = append(lines, Fact{"Environment", d.Environment})
	}
	lines = append(lines, d.Facts...)
	lines = append(lines, Fact{"Received", a.Received.UTC().Format("2006-01-02 15:04 MST")})
	if a.Via != "" {
		lines = append(lines, Fact{"Address", a.Via})
	}
	switch a.ProvenBy {
	case ByApple:
		lines = append(lines, Fact{"Signed", "by Apple"})
	case BySecret:
		lines = append(lines, Fact{"Signed", "with the address's secret"})
	}

	var b strings.Builder
	for _, f := range lines {
		fmt.Fprintf(&b, "%s: %s\n", f.Label, f.Value)
	}
	if m := Meaning(d.Event); m != "" {
		b.WriteString("\n" + m + "\n")
	}
	b.WriteString("\nWhat it said")
	if d.Decoded != nil {
		if pretty, err := json.MarshalIndent(d.Decoded, "", "  "); err == nil {
			if bytes.Contains(body, []byte(`"signedPayload"`)) {
				b.WriteString(", with Apple's signed parts opened up")
			}
			b.WriteString(":\n\n")
			b.Write(pretty)
			b.WriteString("\n")
			return subject, b.String()
		}
	}
	b.WriteString(":\n\n")
	b.WriteString(strings.ToValidUTF8(string(body), "�"))
	return subject, b.String()
}

// notificationFacts are what an App Store Server Notification's data says of the purchase it
// is about, from its transaction and its renewal.
func notificationFacts(data map[string]any) []Fact {
	tx, _ := data["signedTransactionInfo"].(map[string]any)
	renewal, _ := data["signedRenewalInfo"].(map[string]any)
	var facts []Fact
	add := func(label, value string) {
		if value != "" {
			facts = append(facts, Fact{label, value})
		}
	}
	add("Product", text(tx, "productId"))
	add("Type", text(tx, "type"))
	if price, ok := tx["price"].(float64); ok {
		// Apple gives a price in thousandths.
		add("Price", strings.TrimSpace(strconv.FormatFloat(price/1000, 'f', -1, 64)+" "+text(tx, "currency")))
	}
	if q, ok := tx["quantity"].(float64); ok && q > 1 {
		add("Quantity", strconv.FormatFloat(q, 'f', -1, 64))
	}
	add("Purchased", moment(tx["purchaseDate"]))
	add("Expires", moment(tx["expiresDate"]))
	if s, ok := data["status"].(float64); ok {
		add("Status", statuses[int(s)])
	}
	if r, ok := renewal["autoRenewStatus"].(float64); ok {
		add("Auto-renew", map[bool]string{true: "on", false: "off"}[r == 1])
	}
	if next := text(renewal, "autoRenewProductId"); next != "" && next != text(tx, "productId") {
		add("Renews to", next)
	}
	add("Offer", text(tx, "offerIdentifier"))
	add("Storefront", text(tx, "storefront"))
	add("Revoked", moment(tx["revocationDate"]))
	add("Transaction", text(tx, "transactionId"))
	if original := text(tx, "originalTransactionId"); original != text(tx, "transactionId") {
		add("Original transaction", original)
	}
	return facts
}

// statuses are a subscription's, as a notification numbers them.
var statuses = map[int]string{
	1: "active",
	2: "expired",
	3: "in billing retry",
	4: "in grace period",
	5: "revoked",
}

// webhookFacts are what an App Store Connect webhook's attributes say: a state before and after
// as one line, when it happened, and anything else plain enough to read.
func webhookFacts(attrs map[string]any) []Fact {
	var facts []Fact
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	paired := map[string]bool{}
	for _, k := range keys {
		suffix, ok := strings.CutPrefix(k, "new")
		if !ok || suffix == "" {
			continue
		}
		before, after := attrs["old"+suffix], attrs[k]
		if before == nil {
			continue
		}
		label := words(suffix)
		if suffix == "Value" {
			label = "State"
		}
		facts = append(facts, Fact{label, state(before) + " → " + state(after)})
		paired[k], paired["old"+suffix] = true, true
	}
	if at, ok := attrs["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil {
			facts = append(facts, Fact{"At", t.UTC().Format("2006-01-02 15:04 MST")})
		}
	}
	for _, k := range keys {
		if paired[k] || k == "timestamp" {
			continue
		}
		switch v := attrs[k].(type) {
		case string:
			facts = append(facts, Fact{words(k), state(v)})
		case float64, bool:
			facts = append(facts, Fact{words(k), fmt.Sprint(v)})
		}
	}
	return facts
}

// state is a value as a person reads it: READY_FOR_SALE as "Ready for sale", and anything not
// shaped like that as it is.
func state(v any) string {
	s := fmt.Sprint(v)
	if s == "" || strings.ToUpper(s) != s || strings.ContainsAny(s, " .:/") {
		return s
	}
	return words(s)
}

// words spaces out a name, CONSTANT_CASE or camelCase, into a sentence's words, the first
// capitalised: DID_RENEW as "Did renew", buildUploadStateUpdated as "Build upload state updated".
func words(name string) string {
	var parts []string
	if strings.ToUpper(name) == name {
		parts = strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '_' })
	} else {
		start := 0
		runes := []rune(name)
		for i := 1; i < len(runes); i++ {
			if unicode.IsUpper(runes[i]) && !unicode.IsUpper(runes[i-1]) {
				parts = append(parts, strings.ToLower(string(runes[start:i])))
				start = i
			}
		}
		parts = append(parts, strings.ToLower(string(runes[start:])))
	}
	s := strings.Join(parts, " ")
	if s == "" {
		return name
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// moment is a time Apple gives in milliseconds, as a person reads it, "" for none.
func moment(v any) string {
	ms, ok := v.(float64)
	if !ok || ms <= 0 {
		return ""
	}
	return time.UnixMilli(int64(ms)).UTC().Format("2006-01-02 15:04 MST")
}
