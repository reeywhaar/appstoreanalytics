// Package hooks reads what a webhook delivered: what kind of event it is, a line saying what
// happened, and its body with Apple's signed parts opened up to read.
//
// It reads; it never decides whether to believe. A delivery is let in by the secret in its URL,
// and the signatures Apple puts on its payloads are not checked here. See
// docs/app-store-connect.md.
package hooks

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Unknown is the event of a delivery that names none this reads.
const Unknown = "unknown"

// Delivery is what a body says.
type Delivery struct {
	// Event is the kind of event: an App Store Server Notification's type and subtype, like
	// SUBSCRIBED/INITIAL_BUY, an App Store Connect webhook's data type, or Unknown.
	Event string
	// Summary is a line for a list: the event and what it is about, where the payload says.
	Summary string
	// Decoded is the body as JSON with every signed part opened, or nil when it is not JSON.
	Decoded any
}

// Describe reads a body.
func Describe(body []byte) Delivery {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return Delivery{Event: Unknown, Summary: "not JSON"}
	}
	doc = open(doc)
	d := Delivery{Event: Unknown, Decoded: doc}
	top, _ := doc.(map[string]any)
	if top == nil {
		d.Summary = "JSON, not an object"
		return d
	}

	// App Store Server Notifications V2: the whole notification is in signedPayload.
	if payload, ok := top["signedPayload"].(map[string]any); ok {
		kind := text(payload, "notificationType")
		if sub := text(payload, "subtype"); sub != "" {
			kind += "/" + sub
		}
		if kind != "" {
			d.Event = kind
		}
		data, _ := payload["data"].(map[string]any)
		d.Summary = join(d.Event, text(data, "bundleId"), text(data, "environment"))
		return d
	}

	// App Store Connect webhooks: a JSON:API document whose data names its type.
	if data, ok := top["data"].(map[string]any); ok && text(data, "type") != "" {
		d.Event = text(data, "type")
		d.Summary = join(d.Event, text(data, "id"))
		return d
	}

	for _, key := range []string{"type", "event", "eventType", "notificationType"} {
		if v := text(top, key); v != "" {
			d.Event = v
			break
		}
	}
	d.Summary = d.Event
	return d
}

// open replaces every JWS it finds with the claims inside it, so that a signed payload, and the
// signed transaction and renewal inside that, read as plain JSON.
func open(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, inner := range v {
			v[k] = open(inner)
		}
		return v
	case []any:
		for i, inner := range v {
			v[i] = open(inner)
		}
		return v
	case string:
		if claims, ok := jws(v); ok {
			return open(claims)
		}
	}
	return v
}

// jws reads the claims of a compact JWS, unverified, and false for a string that is not one.
func jws(s string) (any, bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 || len(parts[1]) < 2 || !strings.HasPrefix(s, "eyJ") {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims map[string]any
	if json.Unmarshal(raw, &claims) != nil {
		return nil, false
	}
	return claims, true
}

func text(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func join(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}
