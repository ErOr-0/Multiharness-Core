package workflow

import "fmt"

// Dependencies contains the required outbound ports for Service.
type Dependencies struct {
	Workspace   Workspace
	Planner     Planner
	Implementer Implementer
	Validator   Validator
	Reviewer    Reviewer
	Events      EventSink
	Execution   ExecutionPolicy
	RetryWaiter RetryWaiter
	Fallbacks   BillingFallbacks
}

// DependencyError identifies a missing required Service dependency.
type DependencyError struct {
	Name string
}

func (err *DependencyError) Error() string {
	return fmt.Sprintf("workflow dependency %q is required", err.Name)
}

// Service coordinates one workflow run using injected ports. It is safe for
// concurrent use when its injected dependencies are safe for concurrent use.
type Service struct {
	workspace   Workspace
	planner     Planner
	implementer Implementer
	validator   Validator
	reviewer    Reviewer
	events      EventSink
	execution   ExecutionPolicy
	retryWaiter RetryWaiter
	fallbacks   BillingFallbacks
}

// NewService validates dependencies and returns an immutable workflow service.
func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Workspace == nil {
		return nil, &DependencyError{Name: "workspace"}
	}
	if dependencies.Planner == nil {
		return nil, &DependencyError{Name: "planner"}
	}
	if dependencies.Implementer == nil {
		return nil, &DependencyError{Name: "implementer"}
	}
	if dependencies.Validator == nil {
		return nil, &DependencyError{Name: "validator"}
	}
	if dependencies.Reviewer == nil {
		return nil, &DependencyError{Name: "reviewer"}
	}

	events := dependencies.Events
	if err := dependencies.Fallbacks.validate(); err != nil {
		return nil, err
	}
	execution := dependencies.Execution.withDefaults()
	if err := execution.Validate(); err != nil {
		return nil, err
	}
	waiter := dependencies.RetryWaiter
	if waiter == nil {
		waiter = timerWaiter{}
	}
	if events == nil {
		events = discardEventSink{}
	}
	return &Service{
		workspace:   dependencies.Workspace,
		planner:     dependencies.Planner,
		implementer: dependencies.Implementer,
		validator:   dependencies.Validator,
		reviewer:    dependencies.Reviewer,
		events:      events,
		execution:   execution,
		retryWaiter: waiter,
		fallbacks:   dependencies.Fallbacks,
	}, nil
}
