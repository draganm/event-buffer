Feature: send events

    Scenario: send a single event
        When I send a single event
        Then I should get a confirmation
