package claudesdk

import (
	"context"
	"iter"
)

// MessageOrError pairs a Message with an optional error for channel-based consumption.
type MessageOrError struct {
	Message Message
	Err     error
}

// ToChan converts an iter.Seq2[Message, error] into a channel.
// The channel is closed when the iterator is exhausted or the context is cancelled.
//
// On cancellation we attempt to surface ctx.Err() as a final
// MessageOrError but never block on it — a consumer that cancels its
// own context typically stops reading at the same time, so an
// unconditional send would pin this goroutine forever once the
// 8-slot buffer fills. The channel close that follows is the
// authoritative end-of-stream signal.
func ToChan(ctx context.Context, seq iter.Seq2[Message, error]) <-chan MessageOrError {
	ch := make(chan MessageOrError, 8)
	go func() {
		defer close(ch)
		for msg, err := range seq {
			select {
			case <-ctx.Done():
				select {
				case ch <- MessageOrError{Err: ctx.Err()}:
				default:
				}
				return
			case ch <- MessageOrError{Message: msg, Err: err}:
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}
