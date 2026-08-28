package workflow

type TaskInput struct {
	Task            string `json:"task"`
	WorkingDir      string `json:"working_dir"`
	MaxReviewRounds int    `json:"max_review_rounds"`
}

type Plan struct {
	Summary            string   `json:"summary"`
	Steps              []string `json:"steps"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type Review struct {
	Approved    bool     `json:"approved"`
	Summary     string   `json:"summary"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
}

type TaskOutput struct {
	Completed    bool     `json:"completed"`
	Summary      string   `json:"summary"`
	ChangedFiles []string `json:"changed_files"`
	ReviewRounds int      `json:"review_rounds"`
}
