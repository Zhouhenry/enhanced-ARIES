package cloud

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
)

type admissionTestHarness struct {
	unusedHarness
	run  func(context.Context, string) (core.HarnessResult, error)
	stop func(context.Context) error
}

func (h admissionTestHarness) Run(ctx context.Context, instruction string) (core.HarnessResult, error) {
	return h.run(ctx, instruction)
}

func (h admissionTestHarness) Stop(ctx context.Context) error { return h.stop(ctx) }

func TestModelAdmissionClosesAfterNativeRunAndBeforeStop(t *testing.T) {
	for _, runErr := range []error{nil, context.Canceled, errors.New("native run failed")} {
		var order []string
		want := core.HarnessResult{Status: core.StatusFailed, Error: "unchanged native result"}
		h := modelAdmissionHarness{AgentHarness: admissionTestHarness{
			run: func(_ context.Context, instruction string) (core.HarnessResult, error) {
				if instruction != "original task" || len(order) != 0 {
					t.Fatal("admission closed before native Run returned")
				}
				order = append(order, "run_returning")
				return want, runErr
			},
			stop: func(context.Context) error {
				order = append(order, "stop")
				return nil
			},
		}, returned: func() { order = append(order, "admission_closed") }}
		got, err := h.Run(context.Background(), "original task")
		if !reflect.DeepEqual(got, want) || err != runErr {
			t.Fatal("native result or failure changed")
		}
		if err := h.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(order, []string{"run_returning", "admission_closed", "stop"}) {
			t.Fatal(order)
		}
	}
}
