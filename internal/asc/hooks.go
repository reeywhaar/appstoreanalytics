package asc

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AppURLs is where an app's App Store Server Notifications go, "" for nowhere.
type AppURLs struct {
	ID         string
	Name       string
	BundleID   string
	Production string
	Sandbox    string
}

// NotificationURLs is every app of the account with where its server notifications go.
func (c *Client) NotificationURLs(ctx context.Context) ([]AppURLs, error) {
	q := url.Values{}
	q.Set("fields[apps]", "name,bundleId,subscriptionStatusUrl,subscriptionStatusUrlForSandbox")
	q.Set("limit", "200")
	next := "/v1/apps?" + q.Encode()
	var out []AppURLs
	for next != "" {
		var page struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name       string `json:"name"`
					BundleID   string `json:"bundleId"`
					Production string `json:"subscriptionStatusUrl"`
					Sandbox    string `json:"subscriptionStatusUrlForSandbox"`
				} `json:"attributes"`
			} `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := c.get(ctx, next, &page); err != nil {
			return nil, err
		}
		for _, d := range page.Data {
			a := d.Attributes
			out = append(out, AppURLs{ID: d.ID, Name: a.Name, BundleID: a.BundleID, Production: a.Production, Sandbox: a.Sandbox})
		}
		next = page.Links.Next
	}
	return out, nil
}

// Webhook is one of an app's App Store Connect webhooks.
type Webhook struct {
	ID      string
	Name    string
	URL     string
	Enabled bool
	// Events are the kinds of event it sends, in the API's names.
	Events []string
}

// Every reports whether it sends every event there is to send.
func (w Webhook) Every() bool {
	for _, e := range WebhookEvents {
		found := false
		for _, has := range w.Events {
			found = found || has == e
		}
		if !found {
			return false
		}
	}
	return true
}

// Webhooks is an app's webhooks.
func (c *Client) Webhooks(ctx context.Context, appID string) ([]Webhook, error) {
	q := url.Values{}
	q.Set("fields[webhooks]", "name,url,enabled,eventTypes")
	q.Set("limit", "200")
	next := "/v1/apps/" + url.PathEscape(appID) + "/webhooks?" + q.Encode()
	var out []Webhook
	for next != "" {
		var page struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name       string   `json:"name"`
					URL        string   `json:"url"`
					Enabled    bool     `json:"enabled"`
					EventTypes []string `json:"eventTypes"`
				} `json:"attributes"`
			} `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := c.get(ctx, next, &page); err != nil {
			return nil, err
		}
		for _, d := range page.Data {
			a := d.Attributes
			out = append(out, Webhook{ID: d.ID, Name: a.Name, URL: a.URL, Enabled: a.Enabled, Events: a.EventTypes})
		}
		next = page.Links.Next
	}
	return out, nil
}

// WebhookEvents is every event an App Store Connect webhook can be made to send, in the names the
// API takes, as of November 2026: the catalog's webhook kinds, but for the Test button's ping,
// which a webhook is sent whatever it was made for.
var WebhookEvents = []string{
	"APP_STORE_VERSION_APP_VERSION_STATE_UPDATED",
	"BUILD_UPLOAD_STATE_UPDATED",
	"BUILD_BETA_DETAIL_EXTERNAL_BUILD_STATE_UPDATED",
	"BETA_FEEDBACK_SCREENSHOT_SUBMISSION_CREATED",
	"BETA_FEEDBACK_CRASH_SUBMISSION_CREATED",
	"BACKGROUND_ASSET_VERSION_STATE_UPDATED",
	"BACKGROUND_ASSET_VERSION_INTERNAL_BETA_RELEASE_CREATED",
	"BACKGROUND_ASSET_VERSION_EXTERNAL_BETA_RELEASE_STATE_UPDATED",
	"BACKGROUND_ASSET_VERSION_APP_STORE_RELEASE_STATE_UPDATED",
	"ALTERNATIVE_DISTRIBUTION_PACKAGE_VERSION_CREATED",
	"ALTERNATIVE_DISTRIBUTION_PACKAGE_AVAILABLE_UPDATED",
	"ALTERNATIVE_DISTRIBUTION_TERRITORY_AVAILABILITY_UPDATED",
}

// MakeWebhook makes an app's webhook, turned on, sending every event to address signed with
// secret. It takes the Admin or App Manager role.
func (c *Client) MakeWebhook(ctx context.Context, appID, name, address, secret string) (Webhook, error) {
	body := map[string]any{"data": map[string]any{
		"type": "webhooks",
		"attributes": map[string]any{
			"name": name, "url": address, "secret": secret, "enabled": true, "eventTypes": WebhookEvents,
		},
		"relationships": map[string]any{"app": map[string]any{
			"data": map[string]string{"type": "apps", "id": appID},
		}},
	}}
	var made struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.post(ctx, "/v1/webhooks", body, &made); err != nil {
		return Webhook{}, err
	}
	return Webhook{ID: made.Data.ID, Name: name, URL: address, Enabled: true, Events: WebhookEvents}, nil
}

// MendWebhook makes a webhook send every event to address signed with secret, turned on: one
// turned off, sending only some events, or to an address that is gone.
func (c *Client) MendWebhook(ctx context.Context, webhookID, address, secret string) error {
	body := map[string]any{"data": map[string]any{
		"type": "webhooks",
		"id":   webhookID,
		"attributes": map[string]any{
			"url": address, "secret": secret, "enabled": true, "eventTypes": WebhookEvents,
		},
	}}
	return c.send(ctx, http.MethodPatch, "/v1/webhooks/"+url.PathEscape(webhookID), body, nil)
}

// DeleteWebhook deletes a webhook, which stops what it sends.
func (c *Client) DeleteWebhook(ctx context.Context, webhookID string) error {
	return c.send(ctx, http.MethodDelete, "/v1/webhooks/"+url.PathEscape(webhookID), nil, nil)
}

// SetNotificationURL points an app's App Store Server Notifications for production or for
// sandbox at an address, version 2, or at nothing for "".
func (c *Client) SetNotificationURL(ctx context.Context, appID string, sandbox bool, address string) error {
	key, version := "subscriptionStatusUrl", "subscriptionStatusUrlVersion"
	if sandbox {
		key, version = "subscriptionStatusUrlForSandbox", "subscriptionStatusUrlVersionForSandbox"
	}
	attrs := map[string]any{key: nil}
	if address != "" {
		attrs[key], attrs[version] = address, "V2"
	}
	body := map[string]any{"data": map[string]any{"type": "apps", "id": appID, "attributes": attrs}}
	return c.send(ctx, http.MethodPatch, "/v1/apps/"+url.PathEscape(appID), body, nil)
}

// PingWebhook asks App Store Connect to send a webhook its test event, as the Test button on
// the webhook's page does. It arrives as webhookPingCreated.
func (c *Client) PingWebhook(ctx context.Context, webhookID string) error {
	body := map[string]any{"data": map[string]any{
		"type": "webhookPings",
		"relationships": map[string]any{"webhook": map[string]any{
			"data": map[string]string{"type": "webhooks", "id": webhookID},
		}},
	}}
	var created json.RawMessage
	return c.post(ctx, "/v1/webhookPings", body, &created)
}

// ErrNoIAPKey is asking for a test notification without the In-App Purchase key it is signed
// with.
var ErrNoIAPKey = errors.New("no In-App Purchase key is set")

// TestNotification asks the App Store Server API to send an app its test notification, which
// goes to the app's sandbox URL or its production one by which is asked, and arrives as TEST.
// It answers with the token Apple reports how the sending went under.
func (c *Client) TestNotification(ctx context.Context, bundleID string, sandbox bool) (string, error) {
	if c.cred.IAPKey == nil {
		return "", ErrNoIAPKey
	}
	base := c.cred.StoreKitURL
	if sandbox {
		base = c.cred.StoreKitSandboxURL
	}
	token, err := signIAP(c.cred.IssuerID, c.cred.IAPKeyID, c.cred.IAPKey, bundleID, c.now())
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(base, "/")+"/inApps/v1/notifications/test", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode/100 != 2 {
		// The App Store Server API's refusals are its own shape, not App Store Connect's.
		var doc struct {
			ErrorMessage string `json:"errorMessage"`
		}
		why := http.StatusText(resp.StatusCode)
		if json.Unmarshal(raw, &doc) == nil && doc.ErrorMessage != "" {
			why = doc.ErrorMessage
		}
		if resp.StatusCode == http.StatusUnauthorized {
			why = "it refused the In-App Purchase key: check its id, the issuer id and the .p8"
		}
		return "", fmt.Errorf("the App Store Server API answered %d: %s", resp.StatusCode, why)
	}
	var out struct {
		Token string `json:"testNotificationToken"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("read the test notification's answer: %w", err)
	}
	return out.Token, nil
}

// signIAP makes the JWT the App Store Server API takes: App Store Connect's, signed with the In-App
// Purchase key, and naming the app it is about.
func signIAP(issuer, keyID string, key *ecdsa.PrivateKey, bundleID string, now time.Time) (string, error) {
	return jwt(keyID, key, map[string]any{
		"iss": issuer,
		"iat": now.Unix(),
		"exp": now.Add(tokenLifetime).Unix(),
		"aud": "appstoreconnect-v1",
		"bid": bundleID,
	})
}
