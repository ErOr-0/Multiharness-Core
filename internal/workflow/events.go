package workflow

import "multiharness-core/internal/store"

// EventType identifies a structured workflow lifecycle event.
type EventType string

const (
	EventTypeStageStarted        EventType = "stage_started"
	EventTypeStageProgress       EventType = "stage_progress"
	EventTypeStageCompleted      EventType = "stage_completed"
	EventTypeStageFailed         EventType = "stage_failed"
	EventTypeWorkflowCompleted   EventType = "workflow_completed"
	EventTypeAgentRetryScheduled EventType = "agent_retry_scheduled"
	EventTypeAgentSwitched       EventType = "agent_switched"
)

// Event reports workflow progress without requiring consumers to parse text.
type Event struct {
	Sequence         int                       `json:"sequence"`
	Type             EventType                 `json:"type"`
	Stage            store.WorkflowStage       `json:"stage"`
	Status           store.TaskStatus          `json:"status,omitempty"`
	FailureCode      store.FailureCode         `json:"failure_code,omitempty"`
	RepairAttempt    int                       `json:"repair_attempt,omitempty"`
	BlockingFindings int                       `json:"blocking_findings,omitempty"`
	RetryAttempt     int                       `json:"retry_attempt,omitempty"`
	RetryDelayMillis int64                     `json:"retry_delay_millis,omitempty"`
	AgentInvocations int                       `json:"agent_invocations,omitempty"`
	ProviderKind     store.ProviderFailureKind `json:"provider_kind,omitempty"`
}

// EventSink receives synchronous workflow events. Implementations should
// return quickly and must be safe for the Service concurrency they are used in.
type EventSink interface {
	Publish(event Event)
}

type discardEventSink struct{}

func (discardEventSink) Publish(Event) {}

type eventEmitter struct {
	sink     EventSink
	sequence int
}

func newEventEmitter(sink EventSink) *eventEmitter {
	if sink == nil {
		sink = discardEventSink{}
	}
	return &eventEmitter{sink: sink}
}

func (emitter *eventEmitter) publish(event Event) {
	emitter.sequence++
	event.Sequence = emitter.sequence
	emitter.sink.Publish(event)
}

func (emitter *eventEmitter) stageStarted(stage store.WorkflowStage, repairAttempt int) {
	emitter.publish(Event{Type: EventTypeStageStarted, Stage: stage, RepairAttempt: repairAttempt})
}

func (emitter *eventEmitter) stageProgress(stage store.WorkflowStage, repairAttempt, blockingFindings int) {
	emitter.publish(Event{
		Type:             EventTypeStageProgress,
		Stage:            stage,
		RepairAttempt:    repairAttempt,
		BlockingFindings: blockingFindings,
	})
}

func (emitter *eventEmitter) stageCompleted(stage store.WorkflowStage, repairAttempt int) {
	emitter.publish(Event{Type: EventTypeStageCompleted, Stage: stage, RepairAttempt: repairAttempt})
}

func (emitter *eventEmitter) stageFailed(
	stage store.WorkflowStage,
	status store.TaskStatus,
	code store.FailureCode,
	repairAttempt int,
) {
	emitter.publish(Event{
		Type:          EventTypeStageFailed,
		Stage:         stage,
		Status:        status,
		FailureCode:   code,
		RepairAttempt: repairAttempt,
	})
}

func (emitter *eventEmitter) workflowCompleted(stage store.WorkflowStage, status store.TaskStatus) {
	emitter.publish(Event{Type: EventTypeWorkflowCompleted, Stage: stage, Status: status})
}
