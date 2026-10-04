@contract
Feature: Delegate a task through the real command-line application
  The executable provider below is a controlled protocol fixture, not an AI.
  Every scenario launches the built application and an actual child process.
  This verifies configuration, process lifetime, output and filesystem effects.

  Scenario Outline: The default mode delegates once using configured native settings
    Given a disposable workspace configured for "<provider>"
    When I submit "write exactly this task, without a workflow schema"
    Then the command exits with 0 and status "responded"
    And exactly one provider process received the original task and configured settings
    And the provider edit exists in the chosen workspace
    And the result contains the native response without team evidence
    When I submit a follow-up using the returned session
    Then the command exits with 0 and status "responded"
    And the second process resumes the exact first session

    Examples:
      | provider |
      | codex    |
      | claude   |

  Scenario Outline: Codex and Claude permissions reach new and resumed native processes
    Given a disposable workspace configured for "<provider>"
    And the agent setting "<setting>" is "<value>"
    When I submit "perform an operation under the chosen permission mode"
    Then the command exits with 0 and status "responded"
    When I submit a follow-up using the returned session
    Then the command exits with 0 and status "responded"
    And the second process resumes the exact first session
    And each native invocation contains <arguments>

    Examples:
      | provider | setting           | value              | arguments                                         |
      | codex    | sandbox           | workspace-write    | ["-c", "sandbox_mode=\"workspace-write\""]          |
      | codex    | sandbox           | read-only          | ["-c", "sandbox_mode=\"read-only\""]                |
      | codex    | sandbox           | danger-full-access | ["-c", "sandbox_mode=\"danger-full-access\""]       |
      | claude   | permission_policy | reject_on_prompt   | ["--permission-mode", "dontAsk"]                   |
      | claude   | permission_policy | accept_edits       | ["--permission-mode", "acceptEdits"]               |
      | claude   | permission_policy | auto_approve       | ["--permission-mode", "auto"]                      |
      | claude   | permission_policy | bypass_permissions | ["--permission-mode", "bypassPermissions"]         |

  Scenario Outline: A broken native stream cannot become a successful answer
    Given a disposable workspace configured for "<provider>"
    And the provider will "<behavior>"
    When I submit "keep any edit even if the provider fails"
    Then the command exits with 1 and status "failed"
    And the provider edit exists in the chosen workspace
    And no retry or team workflow ran

    Examples:
      | provider | behavior   |
      | codex    | truncated  |
      | claude   | truncated  |
      | claude   | malformed  |
      | codex    | nonzero    |
      | claude   | no-session |

  Scenario Outline: The effective deadline stops the process and preserves partial output
    Given a disposable workspace configured for "codex"
    And the provider will "hang"
    And the "<setting>" deadline is 2 seconds
    When I submit "perform a long operation"
    Then the command exits with 124 and status "timed_out"
    And the result names the "<setting>" deadline and keeps the partial response
    And the provider process stops producing filesystem activity
    And no retry or team workflow ran

    Examples:
      | setting             |
      | timeout             |
      | implementer-timeout |

  Scenario: An explicit permission denial asks for input
    Given a disposable workspace configured for "claude"
    And the provider will "denied"
    When I submit "perform an operation requiring permission"
    Then the command exits with 4 and status "needs_input"
    And no retry or team workflow ran

  Scenario: A provider cannot silently replace the requested conversation
    Given a disposable workspace configured for "codex"
    When I submit "start a conversation"
    Then the command exits with 0 and status "responded"
    And the provider will "different-session"
    When I submit a follow-up using the returned session
    Then the command exits with 1 and status "failed"

  Scenario: Invalid mode is rejected before any provider runs
    Given a disposable workspace configured for "codex"
    And the configured mode is "unknown"
    When I submit "do not execute this invalid configuration"
    Then configuration is rejected before a provider process starts
