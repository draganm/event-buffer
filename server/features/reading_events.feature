Feature: reading events


    Scenario: non-blocking reading of events
        Given one event in the buffer
        When I poll for the events
        Then I should receive the buffered event