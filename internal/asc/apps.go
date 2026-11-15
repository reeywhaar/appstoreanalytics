package asc

import (
	"context"
	"net/url"
)

// App is what the apps list says of one.
type App struct {
	ID       string
	Name     string
	BundleID string
	SKU      string
}

// Apps lists every app of the account, following its pages.
func (c *Client) Apps(ctx context.Context) ([]App, error) {
	q := url.Values{}
	q.Set("fields[apps]", "name,bundleId,sku")
	q.Set("limit", "200")
	next := "/v1/apps?" + q.Encode()
	var out []App
	for next != "" {
		var page struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name     string `json:"name"`
					BundleID string `json:"bundleId"`
					SKU      string `json:"sku"`
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
			out = append(out, App{ID: d.ID, Name: d.Attributes.Name, BundleID: d.Attributes.BundleID, SKU: d.Attributes.SKU})
		}
		next = page.Links.Next
	}
	return out, nil
}
