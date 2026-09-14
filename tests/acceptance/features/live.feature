@live
Feature: Complete a real task with the configured native provider
  This opt-in scenario consumes provider usage and requires a real CLI account.
  No provider executable or protocol is replaced. Only a fresh scratch project
  is sent to the provider; source code and private project files are not sent.

  Scenario: The native agent creates working code and continues its conversation
    Given a real native provider selected by an explicit configuration file
    When I ask the agent to implement integer addition and remember a random token
    Then the real command succeeds with one agent invocation
    And the generated code passes independent positive negative and zero checks
    And the token is absent from project files
    When I ask the same native session to recall the token without reading files
    Then the real command succeeds with one agent invocation
    And the exact token and session are retained
