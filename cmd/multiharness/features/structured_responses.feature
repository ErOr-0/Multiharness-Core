Feature: Ambiguous structured agent decisions never become approval
  Background:
    Given an isolated repository with pre-existing user notes

  Scenario Outline: Reject ambiguous or noncanonical response fields
    Given the "<stage>" response contains a "<issue>"
    When the workflow runs
    Then the result is "failed" with exit code 1
    And no agent ran after "<stage>"
    And no switch is recorded

    Examples:
      | stage          | issue          |
      | planning       | duplicate key  |
      | planning       | wrong key case |
      | implementation | duplicate key  |
      | implementation | wrong key case |
      | review         | duplicate key  |
      | review         | wrong key case |
      | repair         | duplicate key  |
      | repair         | wrong key case |
