@contract
Feature: Team mode distinguishes native permission blocks from invalid responses
  Scenario: Denied module-cache reads stop the workflow until permission is changed
    Given a Team workspace replaying OpenCode's denied module-cache reads
    When I submit "read the reference and finish provider-edit.txt"
    Then the command exits with 4 and status "needs_input"
    And the team identifies the denied read and retains partial files without validation or review
    When I allow implementation permissions and resubmit the original task
    Then the command exits with 0 and status "approved"
    And the team validates the actual file and reviews it after the permission change
