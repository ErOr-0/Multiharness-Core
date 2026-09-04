Feature: Provider failures cannot approve work or cause uncontrolled retries
  As an operator of a commercial coding service
  I need billing failures to stop safely with actionable diagnostics
  And temporary failures to respect explicit retry and execution limits

  Background:
    Given an isolated repository with pre-existing user notes
    And 2 transient retries are allowed

  Scenario Outline: Exhausted billing stops every agent stage
    Given the "<stage>" provider reports "insufficient_quota" for 1 invocations
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the provider failure is "billing_exhausted" after 1 attempts
    And the "<stage>" agent was invoked 1 times
    And no agent ran after "<stage>"

    Examples:
      | stage          |
      | planning       |
      | implementation |
      | review         |
      | repair         |

  Scenario Outline: Billing and access errors are not transient rate limits
    Given the "planning" provider reports "<code>" for 1 invocations
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the provider failure is "<kind>" after 1 attempts
    And the "planning" agent was invoked 1 times
    Examples:
      | code                              | kind                  |
      | credit_balance_exhausted          | billing_exhausted     |
      | organization_spend_limit_exceeded | billing_exhausted     |
      | project_spend_limit_exceeded      | billing_exhausted     |
      | organization_usage_limit_exceeded | billing_exhausted     |
      | invalid_api_key                   | authentication_failed |
      | model_not_found                   | access_denied         |
      | unknown_429                       | unknown               |

  Scenario Outline: Transient read-only failures can recover
    Given the "<stage>" provider reports "<code>" for 1 invocations
    When the workflow runs
    Then the result is "approved" with exit code 0
    And the "<stage>" agent was invoked 2 times
    And 1 retry events were logged
    Examples:
      | stage    | code                 |
      | planning | rate_limit_exceeded  |
      | review   | server_is_overloaded |

  Scenario: Repeated overload exhausts retries without approval
    Given the "planning" provider reports "server_is_overloaded" for 9 invocations
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the provider failure is "overloaded" after 3 attempts
    And the "planning" agent was invoked 3 times

  Scenario: Automatic retries remain opt-in
    Given 0 transient retries are allowed
    And the "planning" provider reports "rate_limit_exceeded" for 1 invocations
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the "planning" agent was invoked 1 times

  Scenario Outline: Mutating work is not replayed after provider failure
    Given the "<stage>" provider reports "rate_limit_exceeded" for 1 invocations
    And the agent edits a file before failing
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the "<stage>" agent was invoked 1 times
    And the partial file remains in independent repository evidence
    Examples:
      | stage          |
      | implementation |
      | repair         |

  Scenario Outline: Provider error transport does not hide exhausted billing
    Given the "<stage>" provider reports "insufficient_quota" for 1 invocations
    And the error transport is "<transport>"
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the provider failure is "billing_exhausted" after 1 attempts
    Examples:
      | stage          | transport         |
      | implementation | no_session        |
      | implementation | error_then_success|
      | implementation | nonzero           |
      | planning       | stderr            |
      | planning       | hanging           |

  Scenario: Retry-After is not shortened to fit our retry budget
    Given the "planning" provider reports "rate_limit_exceeded" for 1 invocations
    And the provider requests a 60 second wait
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the "planning" agent was invoked 1 times
    And 0 retry events were logged

  Scenario: Agent invocation budget includes retries
    Given the "planning" provider reports "rate_limit_exceeded" for 9 invocations
    And the agent invocation limit is 2
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the "planning" agent was invoked 2 times
    And the result records 2 agent invocations

  Scenario: Agent invocation budget prevents downstream execution
    Given the agent invocation limit is 1
    When the workflow runs
    Then the result is "failed" with exit code 1
    And the failure code is "invocation_limit_reached"
    And the "implementation" agent was invoked 0 times

  Scenario: Unsupported monetary guarantees fail before any agent starts
    Given a monetary cap of 1000000 microdollars is required
    When the workflow runs
    Then the result is "failed" with exit code 2
    And the "planning" agent was invoked 0 times
