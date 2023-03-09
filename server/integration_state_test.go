package server_test

import (
	"github.com/draganm/event-buffer/client"
)

type StateKeyType string

const stateKey = StateKeyType("")

type eventsOrError struct {
	events []string
	err    error
}

type State struct {
	// serverBaseURL    string
	client             *client.Client
	pollResult         []string
	secondPollResult   []string
	longPollResult     chan eventsOrError
	longPollResultDesc chan eventsOrError
	lastId             string
}
