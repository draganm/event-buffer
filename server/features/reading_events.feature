Feature: reading events


    Scenario: non-blocking reading of events
        Given one event in the buffer
        When I poll for the events
        Then I should receive the buffered event

    Scenario: blocking reading of events
        Given no events in the buffer
        When I start polling for the events
        And there is a new event sent to the buffer
        Then I should receive the new event