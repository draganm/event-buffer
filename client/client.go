package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type Client struct {
	eventsURL *url.URL
}

func New(baseURL string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("could not parse base URL: %w", err)
	}
	eventsURL := u.JoinPath("events")

	return &Client{eventsURL: eventsURL}, nil

}

func (c *Client) SendEvents(ctx context.Context, events []any) error {

	d, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("could not marshal events: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.eventsURL.String(), bytes.NewReader(d))
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}

	req.Header.Set("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not perform request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		rd, _ := io.ReadAll(res.Body)
		return fmt.Errorf("unexpected status %s: %s", res.Status, string(rd))
	}

	return nil
}

type event struct {
	ID      string
	Payload json.RawMessage
}

func (e *event) UnmarshalJSON(p []byte) error {
	parts := []json.RawMessage{}
	err := json.Unmarshal(p, &parts)

	if err != nil {
		return fmt.Errorf("could not unmarshal parts: %w", err)
	}

	if len(parts) != 2 {
		return fmt.Errorf("expected 2 parts, got %d", len(parts))
	}
	var id string
	err = json.Unmarshal(parts[0], &id)

	if err != nil {
		return fmt.Errorf("could not unmarshal id part: %w", err)
	}

	e.ID = id
	e.Payload = parts[1]

	return nil
}

var errTimeout = errors.New("timeout")

func (c *Client) PollForEvents(ctx context.Context, lastID string, limit int, evts any) ([]string, error) {
	for {
		ids, err := c.pollForEvents(ctx, lastID, limit, evts)

		if err == errTimeout {
			continue
		}

		if err != nil {
			return nil, err
		}

		return ids, nil
	}
}

func (c *Client) pollForEvents(ctx context.Context, lastID string, limit int, evts any) ([]string, error) {
	uc := *c.eventsURL

	u := &uc
	q := u.Query()
	q.Set("limit", strconv.FormatInt(int64(limit), 10))
	q.Set("after", lastID)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not perform request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode == http.StatusRequestTimeout {
		return nil, errTimeout
	}

	if res.StatusCode != http.StatusOK {
		rd, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("unexpected status %s: %s", res.Status, string(rd))
	}

	resp := []event{}
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return nil, fmt.Errorf("could not decode response: %w", err)
	}

	payloads := make([]json.RawMessage, len(resp))

	for i, e := range resp {
		payloads[i] = e.Payload
	}

	d, err := json.Marshal(payloads)
	if err != nil {
		return nil, fmt.Errorf("could not marshal payloads: %w", err)
	}

	err = json.Unmarshal(d, evts)

	if err != nil {
		return nil, fmt.Errorf("could not unmarshal events: %w", err)
	}

	ids := make([]string, len(resp))
	for i, evt := range resp {
		ids[i] = evt.ID
	}

	return ids, nil
}
