Feature: Readable live progress preserves machine output and workflow decisions
  As a CLI user
  I need understandable stage and activity updates
  Without leaking provider data or mistaking activity for approval

  Background:
    Given an isolated repository with pre-existing user notes

  Scenario: Plain readable progress explains a repair cycle
    Given stderr uses "text" logs with "plain" progress and "never" color
    And the initial implementation requires repair
    When the workflow runs
    Then the result is "approved" with exit code 0
    And the readable progress includes "Codex planning"
    And the readable progress includes "OpenCode implementing"
    And the readable progress includes "repair round 1"
    And the readable progress includes "Latest validation: 1 passed, 0 failed"
    And the progress output contains no terminal escapes
    And the result records 5 agent invocations

  Scenario: JSON logs stay machine-readable even when colors are forced
    Given stderr uses "json" logs with "auto" progress and "always" color
    When the workflow runs
    Then the result is "approved" with exit code 0
    And safe activity from both agents appears in valid JSONL
    And the progress output contains no terminal escapes
    And the result records 3 agent invocations

  Scenario: Progress can be disabled without disabling the workflow
    Given stderr uses "json" logs with "off" progress and "always" color
    When the workflow runs
    Then the result is "approved" with exit code 0
    And the progress output is empty
    And the result records 3 agent invocations

  Scenario: Completed agent steps do not turn exhausted repairs into success
    Given stderr uses "text" logs with "plain" progress and "never" color
    And every implementation fails validation
    And 0 repair attempts are allowed
    When the workflow runs
    Then the result is "repair_limit_reached" with exit code 3
    And the readable progress includes "Repair limit reached; not approved"
    And the readable progress includes "Latest validation: 0 passed, 1 failed"
    And the progress output contains no terminal escapes
