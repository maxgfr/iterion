package model

import "context"

type iterationCtxKey struct{}

// WithLoopIteration returns a derived context carrying the 0-based loop
// iteration counter for the node about to execute. The runtime calls
// this immediately before Executor.Execute so backends can tag their
// log output as [NodeID#iter/...] for per-(node, iteration) filtering
// in the editor.
func WithLoopIteration(ctx context.Context, iter int) context.Context {
	return context.WithValue(ctx, iterationCtxKey{}, iter)
}

// LoopIterationFromContext returns the loop iteration stamped by
// WithLoopIteration, or 0 when none was set (the natural default for
// nodes outside any loop).
func LoopIterationFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(iterationCtxKey{}).(int); ok {
		return v
	}
	return 0
}
