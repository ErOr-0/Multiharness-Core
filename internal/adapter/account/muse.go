package account

import (
	"context"

	"multiharness-core/internal/adapter/musecli"
)

func checkMuse(ctx context.Context, runner Runner, r Request) Status {
	if _, err := musecli.Executable(r.Executable); err != nil {
		return Status{Detail: "Muse CLI unavailable; install Muse Code and run /login muse"}
	}
	models, err := museModels(ctx, runner, r)
	if err != nil {
		return Status{Detail: "Muse account/catalog check failed; run /login muse and retry /configuration"}
	}
	for _, model := range models {
		if model.ID == r.Model {
			return Status{true, "Model listed by Muse CLI; subscription and remaining allowance are checked when used"}
		}
	}
	return Status{Detail: "Selected model is not listed by Muse; run muse /models, then update /config"}
}
