@contract
Feature: User-selected Team agents share the same result and error contracts
  Scenario Outline: Canonical and compatible versions preserve the complete workflow
    Given a Team workspace using "<provider>" with "<style>" response versions
    When I submit "create provider-edit.txt containing completed"
    Then the command exits with 0 and status "approved"
    And the selected agent completes planning implementation validation and independent review

    Examples:
      | provider | style   |
      | codex    | string  |
      | opencode | string  |
      | claude   | string  |
      | codex    | integer |
      | opencode | integer |
      | claude   | integer |

  Scenario Outline: Version compatibility cannot manufacture an approval
    Given a Team workspace using "<provider>" with "integer" response versions
    And the native reviewer returns "<output>"
    When I submit "create provider-edit.txt containing completed"
    Then the command exits with 1 and status "failed"
    And invalid review evidence cannot become approval or trigger task replay

    Examples:
      | provider | output             |
      | codex    | unknown-version    |
      | opencode | unknown-version    |
      | claude   | unknown-version    |
      | codex    | string-approval    |
      | opencode | string-approval    |
      | claude   | string-approval    |
      | codex    | duplicate-approval |
      | opencode | duplicate-approval |
      | claude   | duplicate-approval |
      | codex    | blocking-approval  |
      | opencode | blocking-approval  |
      | claude   | blocking-approval  |
      | codex    | truncated          |
      | opencode | truncated          |
      | claude   | truncated          |

  Scenario Outline: Unsupported request arguments preserve an actionable bounded diagnostic
    Given a Team workspace using "<provider>" with "string" response versions
    And the native reviewer returns "provider-error"
    When I submit "create provider-edit.txt containing completed"
    Then the command exits with 1 and status "failed"
    And the unsupported parameter is reported without replay switching agents or exposing raw diagnostics

    Examples:
      | provider |
      | codex    |
      | opencode |
      | claude   |
