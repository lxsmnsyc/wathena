package script

import "runtime"

// Thread is a Lua coroutine that Go starts and resumes, such as an NPC
// dialog. A host function pauses it with Call.Yield, and Go resumes it
// later with the player's answer.
//
// A Thread that is garbage collected without Close is released at a later
// call on the state. Its pending to-be-closed variables do not run.
type Thread struct {
	s       *State
	id      int32
	done    bool
	cleanup runtime.Cleanup
}

// NewThread returns a thread that runs fn when first resumed.
func (s *State) NewThread(fn Value) (*Thread, error) {
	if err := s.enter(); err != nil {
		return nil, err
	}
	defer s.leave()
	if err := s.write([]Value{fn}, nil); err != nil {
		return nil, err
	}
	id, err := s.e.op(opNewThread, 0, 0)
	if err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, &Error{Message: "not a function"}
	}
	t := &Thread{s: s, id: id}
	t.cleanup = runtime.AddCleanup(t, func(p pendingRef) { p.s.queue(p.id) }, pendingRef{s, id})
	return t, nil
}

// Done reports whether the thread has finished or failed.
func (t *Thread) Done() bool { return t.done }

// Resume runs the thread until it pauses, returns, or fails. It returns
// the values passed to Yield, or the values the function returned, with
// done set. Each Resume has its own CPU budget. A thread that fails or
// hits a limit is done.
func (t *Thread) Resume(args ...Value) (vals []Value, done bool, err error) {
	if t.done {
		return nil, true, ErrThreadDone
	}
	s := t.s
	if err := s.enter(); err != nil {
		return nil, false, err
	}
	defer s.leave()
	if err := s.write(nil, args); err != nil {
		return nil, false, err
	}
	status, err := s.e.op(opResume, int64(t.id), int64(len(args)))
	if err != nil {
		return nil, false, err
	}
	vals, err = s.results(status)
	if status != statusYield {
		// A finished or failed thread is released now. glue.c has closed
		// a failed one.
		t.release()
		return vals, true, err
	}
	return vals, false, err
}

func (t *Thread) release() {
	t.done = true
	t.cleanup.Stop()
	_, _ = t.s.e.op(opUnref, int64(t.id), 0)
}

// Close ends a paused thread and runs its pending to-be-closed variables,
// as when a player leaves a dialog. Closing a done thread does nothing.
func (t *Thread) Close() error {
	if t.done {
		return nil
	}
	s := t.s
	if s.e == nil {
		t.done = true
		return nil
	}
	if s.busy.Load() == inHost {
		// Closing runs Lua code, which cannot nest in a host function.
		t.done = true
		t.cleanup.Stop()
		s.queue(t.id)
		return nil
	}
	if err := s.enter(); err != nil {
		return err
	}
	defer s.leave()
	t.done = true
	t.cleanup.Stop()
	_, err := s.e.op(opCloseThread, int64(t.id), 0)
	return err
}
