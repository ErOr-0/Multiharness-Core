Feature: Missing CLI dependencies fail safely with actionable setup guidance
  Background:
    Given an isolated repository with pre-existing user notes

  Scenario: Missing Git stops before any agent runs
    Given the configured "git" executable is missing
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the failure explains executable setup without a provider failure
    And the "planning" agent was invoked 0 times
    And the "implementation" agent was invoked 0 times

  Scenario: Missing planner does not become a billing failure
    Given the configured "planner" executable is missing
    And 2 transient retries are allowed
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the failure explains executable setup without a provider failure
    And the "planning" agent was invoked 0 times
    And the "implementation" agent was invoked 0 times
    And 0 retry events were logged
    And no switch is recorded

  Scenario: Answer-only requests do not require coding dependencies
    Given the planner answers without coding
    And the configured "implementer" executable is missing
    And the configured "reviewer" executable is missing
    When the workflow runs
    Then the result is "answered" with exit code 0
    And the operation sequence is "planning"

  Scenario: Missing implementer preserves the checkout and skips review
    Given the configured "implementer" executable is missing
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the failure explains executable setup without a provider failure
    And the operation sequence is "planning"
    And no switch is recorded

  Scenario: Missing reviewer never approves existing implementation
    Given the configured "reviewer" executable is missing
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the failure explains executable setup without a provider failure
    And the operation sequence is "planning,implementation,validation"
    And the independently verified changes contain only "result.txt"
