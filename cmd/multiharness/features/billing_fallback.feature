Feature: A customer may explicitly switch agents when billing is exhausted
  Background:
    Given an isolated repository with pre-existing user notes

  Scenario Outline: Yes transfers only the failed role to the alternate agent
    Given the "<stage>" provider reports "insufficient_quota" for 1 invocations
    And the operator answers "yes" to billing fallback
    When the workflow runs
    Then the result is "approved" with exit code 0
    And the switch records "<from>" to "<to>" during "<stage>"
    And 1 billing prompts were shown
    And the independently verified changes contain only "result.txt"

    Examples:
      | stage          | from     | to       |
      | implementation | OpenCode | Codex    |
      | repair         | OpenCode | Codex    |
      | planning       | Codex    | OpenCode |
      | review         | Codex    | OpenCode |

  Scenario Outline: Missing or negative consent stops without switching
    Given the "implementation" provider reports "insufficient_quota" for 1 invocations
    And the operator answers "<answer>" to billing fallback
    When the workflow runs
    Then the result is "failed" with exit code 1
    And no switch is recorded
    And the "implementation" agent was invoked 1 times

    Examples:
      | answer |
      | no     |
      |        |
      | EOF    |

  Scenario: Partial implementation is continued under the original baseline
    Given the "implementation" provider reports "insufficient_quota" for 1 invocations
    And the agent edits a file before failing
    And the operator answers "yes" to billing fallback
    When the workflow runs
    Then the result is "approved" with exit code 0
    And the switch records "OpenCode" to "Codex" during "implementation"
    And the independently verified changes contain only "result.txt"

  Scenario: The alternate also exhausting credits never causes a switching loop
    Given the "planning" provider reports "insufficient_quota" for 9 invocations
    And the operator answers "yes" to billing fallback
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the "planning" agent was invoked 2 times
    And 1 billing prompts were shown

  Scenario: Disabling fallback never asks or switches
    Given fallback is disabled
    And the "planning" provider reports "insufficient_quota" for 1 invocations
    And the operator answers "yes" to billing fallback
    When the workflow runs
    Then the result is "failed" with exit code 1
    And 0 billing prompts were shown
    And no switch is recorded

  Scenario: Transient throttling is not permission to change providers
    Given the "planning" provider reports "rate_limit_exceeded" for 1 invocations
    And the operator answers "yes" to billing fallback
    When the workflow runs
    Then the result is "failed" with exit code 1
    And 0 billing prompts were shown
    And no switch is recorded

  Scenario: Partial repair is handed off with feedback and original evidence
    Given the "repair" provider reports "insufficient_quota" for 1 invocations
    And the agent edits a file before failing
    And the operator answers "yes" to billing fallback
    When the workflow runs
    Then the result is "approved" with exit code 0
    And the switch records "OpenCode" to "Codex" during "repair"
    And the result records 1 repair attempts
    And the independently verified changes contain only "result.txt"

  Scenario: Read-only mutation cannot be authorized through billing consent
    Given the "planning" provider reports "insufficient_quota" for 1 invocations
    And the agent edits a file before failing
    And the operator answers "yes" to billing fallback
    When the workflow runs
    Then the result is "failed" with exit code 1
    And 0 billing prompts were shown
    And no switch is recorded
    And no agent ran after "planning"

  Scenario: Exhausted launch budget prevents even a confirmed alternate launch
    Given the "planning" provider reports "insufficient_quota" for 1 invocations
    And the agent invocation limit is 1
    And the operator answers "yes" to billing fallback
    When the workflow runs
    Then the result is "failed" with exit code 1
    And 0 billing prompts were shown
    And no switch is recorded
    And the "planning" agent was invoked 1 times
