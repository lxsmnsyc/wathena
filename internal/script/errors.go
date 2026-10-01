package script

import (
	"errors"
	"strings"
)

// Errors that a call can match with errors.Is.
var (
	// ErrCPULimit means the call used up its instruction budget.
	ErrCPULimit = errors.New("script: CPU limit exceeded")
	// ErrMemoryLimit means the state reached its memory limit.
	ErrMemoryLimit = errors.New("script: memory limit exceeded")
	// ErrTimeLimit means the call ran past its wall-clock limit.
	ErrTimeLimit = errors.New("script: time limit exceeded")
	// ErrReentrant means a host function called into its own state.
	ErrReentrant = errors.New("script: a host function cannot call into its own state")
	// ErrClosed means the state, ref, or thread is closed.
	ErrClosed = errors.New("script: closed")
	// ErrExpired means a handle came from an earlier call. Keep it with
	// State.Keep to use it later.
	ErrExpired = errors.New("script: the value belongs to an earlier call")
	// ErrThreadDone means the thread has finished or failed.
	ErrThreadDone = errors.New("script: the thread has finished")
	// ErrBroken means the WebAssembly runtime failed. The state cannot be
	// used again. The cgo build never returns it.
	ErrBroken = errors.New("script: the runtime failed")
)

// limitTexts are the Lua messages of limits, with the position before them.
var limitTexts = []string{"CPU limit exceeded", "time limit exceeded", "not enough memory"}

// Error is an error raised by a script, or a limit that stopped it.
type Error struct {
	// Message is the Lua error message, with the chunk and line when Lua
	// added them.
	Message string
	// Syntax is true when the source did not compile.
	Syntax bool
	limit  error
	host   error
}

func (e *Error) Error() string {
	if e.limit != nil {
		for _, text := range limitTexts {
			if where, ok := strings.CutSuffix(e.Message, text); ok {
				if where = strings.TrimSuffix(where, ": "); where != "" {
					return e.limit.Error() + " at " + where
				}
				return e.limit.Error()
			}
		}
		return e.limit.Error() + ": " + e.Message
	}
	return "script: " + e.Message
}

// Is matches ErrCPULimit, ErrMemoryLimit, and ErrTimeLimit.
func (e *Error) Is(target error) bool {
	return e.limit != nil && target == e.limit
}

// Unwrap returns the error a host function returned, when that error
// stopped the script.
func (e *Error) Unwrap() error {
	return e.host
}
