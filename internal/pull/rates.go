package pull

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RatesURL answers what a US dollar buys in each currency, as of the latest day. It covers
// every currency Apple pays in, and asks for no key.
const RatesURL = "https://open.er-api.com/v6/latest/USD"

// ratesAge is how old rates may be before they are read again. They change once a day.
const ratesAge = 20 * time.Hour

// currencies refreshes the rates proceeds are summed in dollars with.
func (p *Puller) currencies(ctx context.Context) error {
	at, err := p.st.RatesAt(ctx)
	if err != nil {
		return err
	}
	if !at.IsZero() && p.now().Sub(at) < ratesAge {
		return nil
	}
	rates, err := p.rates(ctx)
	if err != nil {
		return explain("Exchange rates could not be read, so proceeds in other currencies than dollars use the last ones read: "+err.Error(), err)
	}
	return p.st.SaveRates(ctx, rates)
}

func fetchRates(ctx context.Context) (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, RatesURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the rates answered %s", resp.Status)
	}
	var doc struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("the rates are not what they should be: %w", err)
	}
	if doc.Result != "success" || len(doc.Rates) == 0 {
		return nil, fmt.Errorf("the rates answered %q", doc.Result)
	}
	return doc.Rates, nil
}
