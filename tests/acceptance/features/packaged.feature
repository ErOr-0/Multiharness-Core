@packaged
Feature: Use the packaged Docker application
  Scenario: A fresh container supports direct delegation and three-field setup
    Given a locally built Docker image selected for acceptance testing
    When I exercise delegation and interactive setup in disposable container mounts
    Then the packaged edit has the application user ownership
    And the terminal completes three-field setup and starts a new conversation

  Scenario: Claude's native permission engine follows terminal choices with a simulated model
    Given a locally built Docker image selected for acceptance testing
    When I exercise the real Claude CLI with a local simulated model and no external network
    Then the native Claude permission engine grants and revokes filesystem access in the same conversation
