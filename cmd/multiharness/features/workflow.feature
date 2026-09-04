Feature: Coding approval requires independent evidence and review
  As a customer requesting changes to my repository
  I need the coding workflow to preserve my existing work
  And to report its true outcome without claiming perfection

  Background:
    Given an isolated repository with pre-existing user notes

  Scenario: Initial implementation passes validation and review
    When the workflow runs
    Then the result is "approved" with exit code 0
    And the operation sequence is "planning,implementation,validation,review"
    And the independently verified changes contain only "result.txt"
    And the result records 3 agent invocations
    And the result records 0 repair attempts

  Scenario: Blocking feedback is repaired and independently reviewed again
    Given the initial implementation requires repair
    When the workflow runs
    Then the result is "approved" with exit code 0
    And the operation sequence is "planning,implementation,validation,review,repair,validation,review"
    And the independently verified changes contain only "result.txt"
    And the result records 5 agent invocations
    And the result records 1 repair attempts

  Scenario: Repeated rejection exhausts repairs without approval
    Given every implementation fails validation
    And 2 repair attempts are allowed
    When the workflow runs
    Then the result is "repair_limit_reached" with exit code 3
    And the "repair" agent was invoked 2 times
    And the "review" agent was invoked 3 times
    And the result records 2 repair attempts

  Scenario: Zero repairs still permits the initial review
    Given every implementation fails validation
    And 0 repair attempts are allowed
    When the workflow runs
    Then the result is "repair_limit_reached" with exit code 3
    And the operation sequence is "planning,implementation,validation,review"
    And the result records 0 repair attempts

  Scenario: Reviewer approval cannot override failing deterministic checks
    Given every implementation fails validation
    And the reviewer incorrectly approves failing validation
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the failure code is "invalid_output"
    And no agent ran after "review"

  Scenario: Non-coding questions finish as answers without implementation
    Given the planner answers without coding
    When the workflow runs
    Then the result is "answered" with exit code 0
    And the operation sequence is "planning"
    And the result records 1 agent invocations
