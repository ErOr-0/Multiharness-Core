@live_genkit
Feature: Delegate a documentation-dependent Go scaffolding task
  The native agent must fetch public Genkit documentation, write a real Go
  project inside its selected directory, and pass an independent Go build/test.

  Scenario: Create a minimal Genkit Go project without model API credentials
    Given a real native provider selected by an explicit configuration file
    When I ask for a minimal Genkit Go scaffold using current documentation
    Then the real command succeeds with one agent invocation
    And the generated project imports Genkit and passes independent Go checks
