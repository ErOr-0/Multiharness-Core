package structured

import _ "embed"

const (
	planSchemaVersion   = "2"
	reviewSchemaVersion = "1"
)

var (
	//go:embed schemas/plan.v2.json
	planSchema []byte

	//go:embed schemas/review.v1.json
	reviewSchema []byte
)

func PlanSchema() []byte   { return append([]byte(nil), planSchema...) }
func ReviewSchema() []byte { return append([]byte(nil), reviewSchema...) }
