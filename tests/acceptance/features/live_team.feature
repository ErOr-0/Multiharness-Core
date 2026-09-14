@live_team
Feature: Complete a Team task using the configured native agents in every role
  This opt-in check consumes real provider usage. No agent role is a fixture.
  Only a synthetic scratch file is sent to the configured providers.

  Scenario: The user's preferred agents complete a real edit and independent review
    Given a real native provider selected by an explicit configuration file
    When I run a scratch Team task with all configured native roles
    Then the configured agents complete planning implementation validation and review
