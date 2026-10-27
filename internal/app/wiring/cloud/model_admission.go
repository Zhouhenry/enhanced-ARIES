package cloud

import (
	"context"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/runner"
)

type modelAdmissionHarness struct {
	runner.AgentHarness
	returned func()
}

func (h modelAdmissionHarness) Run(ctx context.Context, instruction string) (core.HarnessResult, error) {
	result, err := h.AgentHarness.Run(ctx, instruction)
	h.returned()
	return result, err
}
