package store

import "testing"

func TestTeamNeedsInputRequiresBoundedNativeEvidence(t *testing.T) {
	valid := func() TaskOutput {
		return TaskOutput{Status: TaskStatusNeedsInput, Summary: "blocked", Failure: &TaskFailure{Stage: WorkflowStageImplementation, Code: FailureCodePermission, Message: "read denied", Permission: &PermissionDenied{SessionID: "ses", Action: BlockedAction{Tool: "read", Target: "/cache/file"}}}}
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*TaskOutput){
		func(o *TaskOutput) { o.Failure = nil },
		func(o *TaskOutput) { o.Failure.Permission = nil },
		func(o *TaskOutput) { o.Failure.Permission.SessionID = "" },
		func(o *TaskOutput) { o.Failure.Permission.Action.Tool = "" },
		func(o *TaskOutput) { o.Failure.Stage = WorkflowStageValidation },
		func(o *TaskOutput) { o.Failure.Code = FailureCodeAgent },
		func(o *TaskOutput) { o.Status = TaskStatusApproved },
	} {
		out := valid()
		change(&out)
		if out.Validate() == nil {
			t.Fatal(out)
		}
	}
}
