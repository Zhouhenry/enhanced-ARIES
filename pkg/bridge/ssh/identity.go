package ssh

import (
	"errors"
	"sync"

	"github.com/hyscale-lab/aries/pkg/bridge/ssh/execidentity"
)

const maxIdentityReservations = 65536

// Reservations survive connection loss until this occurrence bridge stops.
type identityRegistry struct {
	mu          sync.Mutex
	limit       int
	invocations map[string]struct{}
	requests    map[string]struct{}
}

func (r *identityRegistry) reserve(m execidentity.Metadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.invocations[m.NativeInvocationID]; exists {
		return errors.New("native invocation already reserved")
	}
	if _, exists := r.requests[m.SSHRequestID]; exists {
		return errors.New("SSH request already reserved")
	}
	if len(r.invocations) >= r.limit {
		return errors.New("tool identity reservation capacity exhausted")
	}
	if r.invocations == nil {
		r.invocations = make(map[string]struct{})
		r.requests = make(map[string]struct{})
	}
	r.invocations[m.NativeInvocationID] = struct{}{}
	r.requests[m.SSHRequestID] = struct{}{}
	return nil
}
