@live_permission
Feature: Handle native OpenCode permission denial without losing the conversation
  This explicitly selected live test uses an actual OpenCode account, a harmless
  synthetic file outside a disposable project, and the unchanged permission policy.

  Scenario: A denied outside-project read can be followed by an in-project task
    Given a real OpenCode provider and a harmless file outside the selected project
    When I ask the native agent to read that outside file using only its read tool
    Then the command exits with 4 and status "needs_input"
    And the denied native read identifies the synthetic file and keeps the session
    When I tell the same session to skip that file and write only inside the project
    Then the real command succeeds with one agent invocation
    And the native agent writes the requested file without changing permissions
