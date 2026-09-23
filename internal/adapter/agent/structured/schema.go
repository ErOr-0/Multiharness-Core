package structured

import _ "embed"

const (
	planSchemaVersion   = "4"
	reviewSchemaVersion = "1"
)

var (
	//go:embed schemas/plan.v4.json
	planSchema []byte

	//go:embed schemas/review.v1.json
	reviewSchema []byte
)

func PlanSchema() []byte   { return append([]byte(nil), planSchema...) }
func ReviewSchema() []byte { return append([]byte(nil), reviewSchema...) }
