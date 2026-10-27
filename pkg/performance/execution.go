package performance

import "context"

type executionIDKey struct{}

// WithExecutionID carries an accepted execution identity outside command data.
func WithExecutionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionIDKey{}, id)
}

func ExecutionID(ctx context.Context) string {
	id, _ := ctx.Value(executionIDKey{}).(string)
	return id
}
