@live_team_permissions
Feature: Change Team implementation permissions through the terminal
  Scenario: Real OpenCode obeys a terminal permission change before validation and review
    Given a real native provider selected by an explicit configuration file
    When I exercise Team permissions with the real OpenCode implementer and fixture planning and review
    Then the real Team implementer reads and writes only after the terminal permission change
