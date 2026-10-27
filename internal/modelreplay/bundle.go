package modelreplay

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

const BranchProtocol = "jiuwen-tool-branches-v1"
const BranchHeader = "X-Aries-Model-Branch"

type requestReceivedKey struct{}

// Bundle routes native task_tool branches by explicit causal IDs, never prompts.
// Every branch has an independent cursor; one immutable Tape owns all branches.
type Bundle struct {
	root                     *Server
	mu                       sync.Mutex
	active                   sync.WaitGroup
	children                 map[string]*Server
	closing, admissionClosed bool
	closeOnce                sync.Once
	closeErr                 error
	failed                   string
}

func NewBundle(o Options) (*Bundle, error) {
	if o.HistoryDialect != JiuwenHistoryDialect {
		return nil, errors.New("branch protocol requires pinned Jiuwen history")
	}
	if o.Mode == "replay" && (o.Tape == nil || o.Tape.Version != bundleVersion) {
		return nil, errors.New("branch replay requires a complete version 4 tape")
	}
	root, err := newServer(o, true)
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(filepath.Join(o.OutputDir, "branches"), 0700); err != nil {
		_ = root.Close()
		return nil, err
	}
	root.branch = "main"
	return &Bundle{root: root, children: map[string]*Server{}}, nil
}

func (b *Bundle) ReplayPolicy() string    { return b.root.ReplayPolicy() }
func (b *Bundle) ObservationError() error { return b.root.ObservationError() }

func splitModelBranch(branch string) (string, string, bool) {
	parts := strings.Split(branch, "/")
	if len(branch) > 256 || len(parts) < 2 || parts[0] != "main" {
		return "", "", false
	}
	for _, id := range parts[1:] {
		if id == "" {
			return "", "", false
		}
		for _, c := range id {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_.:-", c)) {
				return "", "", false
			}
		}
	}
	return strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1], true
}

func (s *Server) pendingTaskCall(ctx context.Context, id string) bool {
	s.mu.Lock()
	if s.busy {
		settled := s.settled
		s.mu.Unlock()
		// The client can begin the child after DONE, before the parent handler commits.
		select {
		case <-settled:
		case <-ctx.Done():
			return false
		}
		s.mu.Lock()
	}
	defer s.mu.Unlock()
	if s.failed != "" || s.lastTurn == nil {
		return false
	}
	for _, call := range s.lastTurn.toolCalls() {
		if call.ID == id && call.Name == "task_tool" {
			return true
		}
	}
	return false
}

func (b *Bundle) reject(w http.ResponseWriter, code string, status int, received time.Time) {
	observed := b.root.beginRequest(received, "chat_completions")
	failure(w, status, code, "model branch request rejected")
	observed.result("rejected", code, status)
	observed.finish()
}

func (b *Bundle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	received := time.Now()
	r = r.WithContext(context.WithValue(r.Context(), requestReceivedKey{}, received))
	b.mu.Lock()
	if b.closing {
		b.mu.Unlock()
		failure(w, 409, "SESSION_CLOSED", "session closed")
		return
	}
	b.active.Add(1)
	defer b.active.Done()
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), append([]byte("Bearer "), b.root.options.AccessToken...)) != 1 {
		b.mu.Unlock()
		b.reject(w, "UNAUTHORIZED", http.StatusUnauthorized, received)
		return
	}
	if b.admissionClosed {
		b.mu.Unlock()
		b.reject(w, "ADMISSION_CLOSED", 409, received)
		return
	}
	if r.Method == http.MethodGet {
		b.mu.Unlock()
		b.root.ServeHTTP(w, r)
		return
	}
	branch := r.Header.Get(BranchHeader)
	if branch == "main" {
		b.mu.Unlock()
		b.root.ServeHTTP(w, r)
		return
	}
	parentName, callID, valid := splitModelBranch(branch)
	parent := b.root
	if parentName != "main" {
		parent = b.children[parentName]
	}
	ctx, cancel := context.WithTimeout(r.Context(), b.root.options.Timeout)
	defer cancel()
	if !valid || parent == nil || !parent.pendingTaskCall(ctx, callID) {
		b.failed = "UNBOUND_MODEL_BRANCH"
		b.mu.Unlock()
		b.reject(w, "UNSUPPORTED", 409, received)
		return
	}
	child := b.children[branch]
	if child == nil {
		o := b.root.options
		o.HistoryDialect = JiuwenSubagentHistoryDialect
		o.OutputDir = filepath.Join(o.OutputDir, "branches", digest(branch))
		o.ObservationLevel = performance.ObservationL0
		if o.Mode == "replay" {
			tape, exists := b.root.tape.Children[branch]
			if !exists {
				b.failed = "UNRECORDED_MODEL_BRANCH"
				b.mu.Unlock()
				b.reject(w, "REPLAY_DIVERGED", 409, received)
				return
			}
			o.Tape = &tape
		}
		var err error
		child, err = New(o)
		if err != nil {
			b.failed = "MODEL_BRANCH_SETUP_FAILED"
			b.mu.Unlock()
			b.reject(w, "UNSUPPORTED", 409, received)
			return
		}
		child.recordInParent = true
		child.branch, child.boundaries = branch, b.root.boundaries
		b.children[branch] = child
	}
	b.mu.Unlock()
	child.ServeHTTP(w, r)
}

func (b *Bundle) CloseAdmission() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.admissionClosed = true
	b.root.CloseAdmission()
	for _, child := range b.children {
		child.CloseAdmission()
	}
}

func (b *Bundle) Close() error {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closing = true
		b.mu.Unlock()
		b.active.Wait()
		for _, child := range b.children {
			b.closeErr = errors.Join(b.closeErr, child.Close())
		}
		if b.root.options.Mode == "replay" && len(b.children) != len(b.root.tape.Children) {
			b.closeErr = errors.Join(b.closeErr, errors.New("recorded model branches were not all executed"))
		}
		if b.root.options.Mode == "record" {
			b.root.tape.Version = bundleVersion
			b.root.tape.Children = map[string]Tape{}
			for branch, child := range b.children {
				b.root.tape.Children[branch] = child.tape
			}
			b.closeErr = errors.Join(b.closeErr, validateBundleChildren(b.root.tape))
		}
		if b.failed != "" {
			b.closeErr = errors.Join(b.closeErr, errors.New(b.failed))
		}
		if b.closeErr != nil {
			b.root.failed = "CHILD_SESSION_FAILED"
		}
		b.closeErr = errors.Join(b.closeErr, b.root.Close())
	})
	return b.closeErr
}

func validateBundleChildren(root Tape) error {
	if root.HistoryDialect != JiuwenHistoryDialect {
		return errors.New("branch tape requires pinned Jiuwen root history")
	}
	turns, total, frames := len(root.Turns), 0, 0
	countBytes := func(t Tape) {
		for _, turn := range t.Turns {
			total += len(turn.Request) + len(turn.ResponseJSON)
			frames += len(turn.Frames)
			for _, frame := range turn.Frames {
				total += len(frame.Data)
			}
		}
	}
	countBytes(root)
	for branch, child := range root.Children {
		parentName, callID, ok := splitModelBranch(branch)
		parent := root
		if parentName != "main" {
			parent, ok = root.Children[parentName]
		}
		if !ok || child.Version != jsonResponseVersion || len(child.Children) != 0 || child.HistoryDialect != JiuwenSubagentHistoryDialect || child.Identity != root.Identity || child.Model != root.Model || child.ToolCallIDDialect != root.ToolCallIDDialect {
			return errors.New("invalid child trajectory identity or parent")
		}
		matches := 0
		for _, turn := range parent.Turns {
			for _, call := range turn.toolCalls() {
				if call.ID == callID && call.Name == "task_tool" {
					matches++
				}
			}
		}
		if matches != 1 {
			return errors.New("child trajectory does not identify one exact task_tool call")
		}
		if err := validateTape(child); err != nil {
			return err
		}
		turns += len(child.Turns)
		countBytes(child)
	}
	if turns > maxTurns || total > maxTape || frames > maxFrames {
		return errors.New("branch tape exceeds aggregate bound")
	}
	return nil
}

func (t Tape) SessionProtocol() string {
	if t.Version == bundleVersion {
		return BranchProtocol
	}
	return ""
}
