package asc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// How long an analytics report request runs.
const (
	// Ongoing makes a report every day from the day it is asked for.
	Ongoing = "ONGOING"
	// Snapshot makes one report of the history there is, once.
	Snapshot = "ONE_TIME_SNAPSHOT"
)

// ReportRequest is an app's standing request for analytics reports.
type ReportRequest struct {
	ID         string
	AccessType string
	// Stopped is a request Apple ended because nobody read its reports.
	Stopped bool
}

// ReportRequests lists an app's analytics report requests.
func (c *Client) ReportRequests(ctx context.Context, appID string) ([]ReportRequest, error) {
	var out []ReportRequest
	next := "/v1/apps/" + url.PathEscape(appID) + "/analyticsReportRequests?limit=200"
	for next != "" {
		var page struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					AccessType             string `json:"accessType"`
					StoppedDueToInactivity bool   `json:"stoppedDueToInactivity"`
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
			out = append(out, ReportRequest{ID: d.ID, AccessType: d.Attributes.AccessType, Stopped: d.Attributes.StoppedDueToInactivity})
		}
		next = page.Links.Next
	}
	return out, nil
}

// RequestReports asks Apple to make an app's analytics reports. It takes the Admin role.
func (c *Client) RequestReports(ctx context.Context, appID, accessType string) (ReportRequest, error) {
	in := map[string]any{"data": map[string]any{
		"type":       "analyticsReportRequests",
		"attributes": map[string]any{"accessType": accessType},
		"relationships": map[string]any{"app": map[string]any{
			"data": map[string]any{"type": "apps", "id": appID},
		}},
	}}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.post(ctx, "/v1/analyticsReportRequests", in, &out); err != nil {
		return ReportRequest{}, err
	}
	return ReportRequest{ID: out.Data.ID, AccessType: accessType}, nil
}

// DeleteRequest deletes a report request, which stops its reports. It takes the Admin role.
func (c *Client) DeleteRequest(ctx context.Context, requestID string) error {
	body, err := c.do(ctx, http.MethodDelete, "/v1/analyticsReportRequests/"+url.PathEscape(requestID), nil, "application/json")
	if err != nil {
		return err
	}
	return body.Close()
}

// Report is one kind of report a request makes, such as App Downloads Standard.
type Report struct {
	ID   string
	Name string
}

// Reports lists what a request makes.
func (c *Client) Reports(ctx context.Context, requestID string) ([]Report, error) {
	var out []Report
	next := "/v1/analyticsReportRequests/" + url.PathEscape(requestID) + "/reports?limit=200"
	for next != "" {
		var page struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name string `json:"name"`
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
			out = append(out, Report{ID: d.ID, Name: d.Attributes.Name})
		}
		next = page.Links.Next
	}
	return out, nil
}

// ReportInstance is a report as Apple made it on one day.
type ReportInstance struct {
	ID             string
	ProcessingDate string
}

// Instances lists a report's daily instances.
func (c *Client) Instances(ctx context.Context, reportID string) ([]ReportInstance, error) {
	var out []ReportInstance
	next := "/v1/analyticsReports/" + url.PathEscape(reportID) + "/instances?filter[granularity]=DAILY&limit=200"
	for next != "" {
		var page struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					ProcessingDate string `json:"processingDate"`
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
			out = append(out, ReportInstance{ID: d.ID, ProcessingDate: d.Attributes.ProcessingDate})
		}
		next = page.Links.Next
	}
	return out, nil
}

// Segments lists where an instance's files are.
func (c *Client) Segments(ctx context.Context, instanceID string) ([]string, error) {
	var out []string
	next := "/v1/analyticsReportInstances/" + url.PathEscape(instanceID) + "/segments?limit=200"
	for next != "" {
		var page struct {
			Data []struct {
				Attributes struct {
					URL string `json:"url"`
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
			out = append(out, d.Attributes.URL)
		}
		next = page.Links.Next
	}
	return out, nil
}

// Segment opens one of an instance's files. Its address is signed already, and is asked
// without the API's token.
func (c *Client) Segment(ctx context.Context, address string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("a report file answered %s", resp.Status)
	}
	return resp.Body, nil
}
