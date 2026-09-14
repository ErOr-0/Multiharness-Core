@live_codex_permissions_ui
Feature: Configure native Codex permissions from the Multiharness terminal
  Scenario: Change the sandbox without discarding the current Codex conversation
    Given a real native provider selected by an explicit configuration file
    When I change Codex sandbox permissions through the real interactive terminal
    Then the native Codex sandbox allows and prevents the expected filesystem changes
