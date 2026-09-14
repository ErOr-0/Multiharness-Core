@live_permissions_ui
Feature: Configure native OpenCode permissions from the Multiharness terminal
  Scenario: Grant and revoke permission without leaving the blocked conversation
    Given a real native provider selected by an explicit configuration file
    When I change OpenCode permissions through the real interactive terminal
    Then the native CLI grants and revokes access in the same conversation
