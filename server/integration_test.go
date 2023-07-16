package server_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/cucumber/godog"
	"github.com/draganm/event-buffer/client"
	"github.com/draganm/event-buffer/server/testrig"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

const (
	sortDesc = "desc"
	sortAsc  = "asc"
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
	Concurrency:   runtime.NumCPU(),
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

		cl, err := client.New(serverURL)
		if err != nil {
			return ctx, fmt.Errorf("could not create client: %w", err)
		}

		state.client = cl

		ctx = context.WithValue(ctx, stateKey, state)

		return ctx, nil
	})

	ctx.Step(`^I send a single event$`, iSendASingleEvent)
	ctx.Step(`^I should get a confirmation$`, iShouldGetAConfirmation)
	ctx.Step(`^I poll for the events$`, iPollForTheEvents)
	ctx.Step(`^I should receive the buffered event$`, iShouldReceiveTheBufferedEvent)
	ctx.Step("^I should receive the buffered events in desc order$", iShouldReceiveDescTheNewEvent)
	ctx.Step(`^one event in the buffer$`, oneEventInTheBuffer)
	ctx.Step(`^I should receive the new event$`, iShouldReceiveTheNewEvent)
	ctx.Step(`^I start polling for the events$`, iStartPollingForTheEvents)
	ctx.Step(`^no events in the buffer$`, noEventsInTheBuffer)
	ctx.Step(`^there is a new event sent to the buffer$`, thereIsANewEventSentToTheBuffer)
	ctx.Step(`^I poll for one event$`, iPollForOneEvent)
	ctx.Step(`^I poll for other event after the previous event$`, iPollForOtherEventAfterThePreviousEvent)
	ctx.Step(`^I should get one event for each poll$`, iShouldGetOneEventForEachPoll)
	ctx.Step(`^two events in the buffer$`, twoEventsInTheBuffer)

}

func getState(ctx context.Context) *State {
	return ctx.Value(stateKey).(*State)
}

func iSendASingleEvent(ctx context.Context) error {
	s := getState(ctx)
	err := s.client.SendEvents(ctx, []any{"evt1"})
	if err != nil {
		return err
	}
	return nil
}

func iShouldGetAConfirmation() error {
	// actually nothing comes back
	return nil
}

func oneEventInTheBuffer(ctx context.Context) error {
	s := getState(ctx)
	err := s.client.SendEvents(ctx, []any{"evt1"})
	if err != nil {
		return err
	}
	return nil
}

func iPollForTheEvents(ctx context.Context) error {
	s := getState(ctx)
	evts := []string{}
	_, err := s.client.PollForEvents(ctx, "", 100, sortAsc, &evts)
	if err != nil {
		return fmt.Errorf("failed polling for events: %w", err)
	}
	s.pollResult = evts
	return nil
}

func iShouldReceiveTheBufferedEvent(ctx context.Context) error {
	s := getState(ctx)
	d := cmp.Diff(s.pollResult, []string{"evt1"})
	if d != "" {
		return fmt.Errorf("unexpected poll result:\n%s", d)
	}
	return nil
}

func noEventsInTheBuffer() error {
	// this is the case at beginning of a test
	return nil
}

func iStartPollingForTheEvents(ctx context.Context) error {
	s := getState(ctx)
	s.longPollResult = make(chan eventsOrError, 1)
	go func() {
		evts := []string{}
		_, err := s.client.PollForEvents(ctx, "", 100, sortAsc, &evts)
		if err != nil {
			s.longPollResult <- eventsOrError{err: fmt.Errorf("failed polling for events: %w", err)}
			return
		}
		s.longPollResult <- eventsOrError{events: evts}
	}()

	s.longPollResultDesc = make(chan eventsOrError, 1)
	go func() {
		evts := []string{}
		_, err := s.client.PollForEvents(ctx, "", 100, sortDesc, &evts)
		if err != nil {
			s.longPollResult <- eventsOrError{err: fmt.Errorf("failed polling for events: %w", err)}
			return
		}
		s.longPollResultDesc <- eventsOrError{events: evts}
	}()

	return nil
}

func thereIsANewEventSentToTheBuffer(ctx context.Context) error {
	s := getState(ctx)
	err := s.client.SendEvents(ctx, []any{"evt1"})
	if err != nil {
		return err
	}
	return nil
}

func iShouldReceiveTheNewEvent(ctx context.Context) error {
	s := getState(ctx)
	select {
	case <-ctx.Done():
		return fmt.Errorf("could not get long poll event: %w", ctx.Err())
	case res := <-s.longPollResult:
		if res.err != nil {
			return fmt.Errorf("long poll failed: %w", res.err)
		}
		d := cmp.Diff(res.events, []string{"evt1"})

		if d != "" {
			return fmt.Errorf("unexpected poll result:\n%s", d)
		}
	}

	return nil
}

func twoEventsInTheBuffer(ctx context.Context) error {
	s := getState(ctx)
	err := s.client.SendEvents(ctx, []any{"evt1", "evt2"})
	if err != nil {
		return err
	}
	return nil
}

func iPollForOneEvent(ctx context.Context) error {
	s := getState(ctx)
	evts := []string{}
	ids, err := s.client.PollForEvents(ctx, "", 1, sortAsc, &evts)
	if err != nil {
		return fmt.Errorf("failed polling for events: %w", err)
	}

	if len(ids) != 1 {
		return fmt.Errorf("expected 1 event, got %d", len(ids))
	}

	s.pollResult = evts
	s.lastId = ids[len(ids)-1]
	return nil
}

func iPollForOtherEventAfterThePreviousEvent(ctx context.Context) error {
	s := getState(ctx)
	evts := []string{}
	_, err := s.client.PollForEvents(ctx, s.lastId, 1, sortAsc, &evts)
	if err != nil {
		return fmt.Errorf("failed polling for events: %w", err)
	}
	s.secondPollResult = evts
	return nil
}

func iShouldGetOneEventForEachPoll(ctx context.Context) error {
	s := getState(ctx)
	d := cmp.Diff(s.pollResult, []string{"evt1"})
	if d != "" {
		return fmt.Errorf("unexpected poll result:\n%s", d)
	}
	d = cmp.Diff(s.secondPollResult, []string{"evt2"})
	if d != "" {
		return fmt.Errorf("unexpected second poll result:\n%s", d)
	}
	return nil
}

func iShouldReceiveDescTheNewEvent(ctx context.Context) error {
	s := getState(ctx)
	select {
	case <-ctx.Done():
		return fmt.Errorf("could not get long poll event: %w", ctx.Err())
	case res := <-s.longPollResultDesc:
		if res.err != nil {
			return fmt.Errorf("long poll failed: %w", res.err)
		}
		d := cmp.Diff(res.events, []string{"evt1"})

		if d != "" {
			return fmt.Errorf("unexpected poll result:\n%s", d)
		}
	}

	return nil
}
