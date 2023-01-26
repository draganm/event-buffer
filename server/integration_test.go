package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/draganm/event-buffer/server/testrig"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

func init() {
	logger, _ := zap.NewDevelopment()
	if false {
		opts.DefaultContext = logr.NewContext(context.Background(), zapr.NewLogger(logger))
	}
}

var opts = godog.Options{
	Output:        os.Stdout,
	StopOnFailure: true,
	Strict:        true,
	Format:        "progress",
	Paths:         []string{"features"},
	NoColors:      true,
}

func init() {
	godog.BindCommandLineFlags("godog.", &opts)
}

func TestMain(m *testing.M) {
	pflag.Parse()
	opts.Paths = pflag.Args()

	status := godog.TestSuite{
		Name:                "godogs",
		ScenarioInitializer: InitializeScenario,
		Options:             &opts,
	}.Run()

	os.Exit(status)
}

type StateKeyType string

const stateKey = StateKeyType("")

type State struct {
	serverBaseURL string
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

func InitializeScenario(ctx *godog.ScenarioContext) {
	var cancel context.CancelFunc

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		ctx, cancel = context.WithCancel(ctx)

		return ctx, nil

	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		cancel()
		return ctx, nil
	})

	state := &State{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {

		serverURL, err := testrig.StartServer(ctx, logr.FromContextOrDiscard(ctx))
		if err != nil {
			return ctx, fmt.Errorf("could not start server: %w", err)
		}
		state.serverBaseURL = serverURL

		ctx = context.WithValue(ctx, stateKey, state)

		return ctx, nil
	})

	ctx.Step(`^I send a single event$`, iSendASingleEvent)
	ctx.Step(`^I should get a confirmation$`, iShouldGetAConfirmation)

}

func getState(ctx context.Context) *State {
	return ctx.Value(stateKey).(*State)
}

func iSendASingleEvent(ctx context.Context) error {
	s := getState(ctx)
	err := s.sendEvents(ctx, []any{"evt1"})
	if err != nil {
		return err
	}
	return nil
}

func iShouldGetAConfirmation() error {
	// actually nothing comes back
	return nil
}
