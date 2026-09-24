package structured

import (
	"strings"
	"testing"
)

func TestReviewValidationActionProtocol(t *testing.T) {
	payload := `{"schema_version":"2","approved":false,"summary":"needs validation","findings":[{"severity":"error","blocking":true,"file":"","line":0,"description":"cache write blocked","evidence":"permission denied","required_action":"run validation with consent"}],"suggestions":[],"validation_action":{"executable":"go","args":["test","./..."],"reason":"requires writable cache"}}`
	r, err := ParseReview([]byte(payload))
	if err != nil || r.ValidationAction == nil || r.ValidationAction.Executable != "go" {
		t.Fatal(r, err)
	}
	for _, invalid := range []string{
		strings.Replace(payload, `"reason":"requires writable cache"`, `"reason":""`, 1),
		strings.Replace(payload, `"executable":"go"`, `"executable":"go","grant_all":true`, 1),
		strings.Replace(payload, `"approved":false`, `"approved":true`, 1),
	} {
		if _, err := ParseReview([]byte(invalid)); err == nil {
			t.Fatal("accepted invalid action", invalid)
		}
	}
}
