package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type StateKeyType string

const stateKey = StateKeyType("")

type eventsOrError struct {
	events []string
	err    error
}

type State struct {
	serverBaseURL    string
	pollResult       []string
	secondPollResult []string
	longPollResult   chan eventsOrError
	lastId           string
}

func (s *State) sendEvents(ctx context.Context, events []any) error {
	u, err := url.JoinPath(s.serverBaseURL, "events")
	if err != nil {
		return fmt.Errorf("could not join url path: %w", err)
	}

	d, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("could not marshal events: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(d))
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}

	req.Header.Set("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not perform request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
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

func (s *State) pollForEvents(ctx context.Context, evts any, lastID string, limit int) (string, error) {

	u, err := url.Parse(s.serverBaseURL)
	if err != nil {
		return "", fmt.Errorf("could not parse url: %w", err)
	}

	u = u.JoinPath("events")
	q := u.Query()
	q.Set("limit", strconv.FormatInt(int64(limit), 10))
	q.Set("after", lastID)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not perform request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		rd, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("unexpected status %s: %s", res.Status, string(rd))
	}

	resp := []event{}
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return "", fmt.Errorf("could not decode response: %w", err)
	}

	payloads := make([]json.RawMessage, len(resp))

	for i, e := range resp {
		payloads[i] = e.Payload
	}

	d, err := json.Marshal(payloads)
	if err != nil {
		return "", fmt.Errorf("could not marshal payloads: %w", err)
	}

	err = json.Unmarshal(d, evts)

	if err != nil {
		return "", fmt.Errorf("could not unmarshal events: %w", err)
	}

	if len(resp) > 0 {
		return resp[len(resp)-1].ID, nil
	}

	return "", nil

}
