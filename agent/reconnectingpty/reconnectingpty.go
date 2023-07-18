package reconnectingpty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty"
)

const attachTimeout = 30 * time.Second

// Options allows configuring the reconnecting pty.
type Options struct {
	Logger slog.Logger
	// Timeout describes how long to keep the pty alive without any connections.
	// Once elapsed the pty will be killed.
	Timeout time.Duration
	// Metrics tracks various error counters.
	Metrics *prometheus.CounterVec
	// BackendType indicates which backend to use for reconnections.
	BackendType codersdk.ReconnectingPTYBackendType
}

// State represents the current state of the reconnecting pty.  States are
// sequential and will only move forward.
type State int

const (
	// StateStarting is the default/start state.  Attaching will block until the
	// reconnecting pty becomes ready.
	StateStarting = iota
	// StateReady means the reconnecting pty is ready to be attached.
	StateReady
	// StateClosing means the reconnecting pty has begun closing.  The underlying
	// process may still be exiting.  Attaching will result in an error.
	StateClosing
	// StateDone means the reconnecting pty has completely shut down and the
	// process has exited.  Attaching will result in an error.
	StateDone
)

type backend interface {
	start(ctx context.Context) error
	attach(ctx context.Context, connID string, conn net.Conn, height, width uint16) (pty.PTYCmd, error)
	close() error
}

// ReconnectingPTY is a pty that can be reconnected within a timeout and to
// simultaneous connections.
type ReconnectingPTY struct {
	// The reconnecting pty can be backed by screen if installed or a (buggy)
	// buffer replay fallback.
	backend backend
	// cond broadcasts state changes and any accompanying errors.
	cond *sync.Cond
	// error hold any error that occurred during a state change and is returned
	// through Attach.  It is not safe to access outside of cond.L.
	error error
	// options holds options for configuring the reconnecting pty.
	options *Options
	// state holds the current reconnecting pty state.  It is not safe to access
	// this outside of cond.L.
	state State
	// timer will close the reconnecting pty when it expires.  The timer will be
	// reset as long as there are active connections.
	timer *time.Timer
}

// New sets up a new reconnecting pty that wraps the provided command.  Any
// errors with starting are returned on Attach().  The reconnecting pty will
// close itself if nothing is attached for the duration of the timeout or if the
// context ends.
func New(ctx context.Context, cmd *pty.Cmd, options *Options) *ReconnectingPTY {
	if options.Timeout == 0 {
		options.Timeout = 5 * time.Minute
	}
	if options.BackendType == "" || options.BackendType == codersdk.ReconnectingPTYBackendTypeAuto {
		_, err := exec.LookPath("screen")
		if err == nil {
			options.BackendType = codersdk.ReconnectingPTYBackendTypeScreen
		} else {
			options.BackendType = codersdk.ReconnectingPTYBackendTypeBuffered
		}
		options.Logger.Debug(ctx, "auto backend selection", slog.F("backend", options.BackendType))
	}
	rpty := &ReconnectingPTY{
		cond:    sync.NewCond(&sync.Mutex{}),
		options: options,
		state:   StateStarting,
	}
	switch options.BackendType {
	case codersdk.ReconnectingPTYBackendTypeScreen:
		// The screen backend is not passed closeSession because we have no way of
		// knowing when the screen daemon dies externally anyway. The consequence is
		// that we might leave reconnecting ptys in memory around longer than they
		// need to be but they will eventually clean up with the timer or context,
		// or the next attach will respawn the screen daemon which is fine too.
		rpty.backend = &screenBackend{
			command: cmd,
			logger:  rpty.options.Logger,
			metrics: rpty.options.Metrics,
		}
	default:
		rpty.backend = &bufferedBackend{
			command: cmd,
			logger:  rpty.options.Logger,
			metrics: rpty.options.Metrics,
			closeSession: func(reason string) {
				rpty.Close(reason)
			},
		}
	}
	go rpty.lifecycle(ctx)
	return rpty
}

// Attach attaches the provided connection to the pty, waits for the attach to
// complete, then pipes the pty and the connection.  The connection is expected
// to send JSON-encoded messages and accept raw output from the ptty.  If the
// context ends it will terminate the connection.
func (rpty *ReconnectingPTY) Attach(ctx context.Context, connID string, conn net.Conn, height, width uint16) error {
	// This will kill the heartbeat and detach the connection from the backend
	// once we hit EOF on the connection (or an error).
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state, err := rpty.waitForStateOrContext(ctx, StateReady)
	if state != StateReady {
		return xerrors.Errorf("reconnecting pty ready wait: %w", err)
	}

	go rpty.heartbeat(ctx)

	ptty, err := rpty.backend.attach(ctx, connID, conn, height, width)
	if err != nil {
		return xerrors.Errorf("reconnecting pty attach: %w", err)
	}

	decoder := json.NewDecoder(conn)
	var req codersdk.ReconnectingPTYRequest
	for {
		err = decoder.Decode(&req)
		if xerrors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			rpty.options.Logger.Warn(ctx, "reconnecting pty failed with read error", slog.Error(err))
			return nil
		}
		_, err = ptty.InputWriter().Write([]byte(req.Data))
		if err != nil {
			rpty.options.Logger.Warn(ctx, "reconnecting pty failed with write error", slog.Error(err))
			rpty.options.Metrics.WithLabelValues("input_writer").Add(1)
			return nil
		}
		// Check if a resize needs to happen!
		if req.Height == 0 || req.Width == 0 {
			continue
		}
		err = ptty.Resize(req.Height, req.Width)
		if err != nil {
			// We can continue after this, it's not fatal!
			rpty.options.Logger.Error(ctx, "reconnecting pty resize failed, but will continue", slog.Error(err))
			rpty.options.Metrics.WithLabelValues("resize").Add(1)
		}
	}
}

// Wait waits for the reconnecting pty to close.  The underlying process might
// still be exiting.
func (rpty *ReconnectingPTY) Wait() {
	_, _ = rpty.waitForState(StateClosing)
}

// Close attempts to gracefully kill the reconnecting pty's underlying process
// then waits for the process to exit.
func (rpty *ReconnectingPTY) Close(reason string) {
	// The closing state change will be handled by the lifecycle.
	rpty.setState(StateClosing, xerrors.Errorf(fmt.Sprintf("reconnecting pty is closing: %s", reason)))
	_, _ = rpty.waitForState(StateDone)
}

// lifecycle manages the lifecycle of the reconnecting pty.  If the context ends
// the reconnecting pty will be closed.
func (rpty *ReconnectingPTY) lifecycle(ctx context.Context) {
	err := rpty.backend.start(ctx)
	if err != nil {
		rpty.setState(StateDone, xerrors.Errorf("reconnecting pty start: %w", err))
		return
	}

	// The initial timeout for attaching will probably be far shorter than the
	// reconnect timeout in most cases; in tests it might be longer.  It should be
	// at least long enough for the first screen attach to be able to start up the
	// daemon.
	rpty.timer = time.AfterFunc(attachTimeout, func() {
		rpty.Close("reconnecting pty timeout")
	})

	rpty.setState(StateReady, nil)

	state, _ := rpty.waitForStateOrContext(ctx, StateClosing)
	if state < StateClosing {
		// The context is what unblocked us (which means the agent is shutting down)
		// so move into the closing phase.
		rpty.setState(StateClosing, nil)
	}
	rpty.timer.Stop()
	err = rpty.backend.close()
	if err != nil {
		rpty.options.Logger.Error(ctx, "close reconnecting pty", slog.Error(err))
	} else {
		rpty.options.Logger.Debug(ctx, "close reconnecting pty")
	}
	rpty.setState(StateDone, xerrors.Errorf("reconnecting pty is done"))
}

// heartbeat keeps the pty alive while the provided context is not done.
func (rpty *ReconnectingPTY) heartbeat(ctx context.Context) {
	// We just connected so reset the timer now in case it is near the end.
	rpty.timer.Reset(rpty.options.Timeout)

	// Reset when the connection closes to ensure the pty stays up for the full
	// timeout.
	defer rpty.timer.Reset(rpty.options.Timeout)

	heartbeat := time.NewTicker(rpty.options.Timeout / 2)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			rpty.timer.Reset(rpty.options.Timeout)
		}
	}
}

// setState sets and broadcasts the provided state if it is greater than the
// current state and the error if one has not already been set.
func (rpty *ReconnectingPTY) setState(state State, err error) {
	rpty.cond.L.Lock()
	defer rpty.cond.L.Unlock()
	// Cannot regress states.  For example, trying to close after the process is
	// done should leave us in the done state and not the closing state.
	if state <= rpty.state {
		return
	}
	rpty.error = errors.Join(rpty.error, err)
	rpty.state = state
	rpty.cond.Broadcast()
}

// waitForState blocks until the state or a greater one is reached.
func (rpty *ReconnectingPTY) waitForState(state State) (State, error) {
	rpty.cond.L.Lock()
	defer rpty.cond.L.Unlock()
	for state > rpty.state {
		rpty.cond.Wait()
	}
	return rpty.state, rpty.error
}

// waitForStateOrContext blocks until the state or a greater one is reached or
// the provided context ends.
func (rpty *ReconnectingPTY) waitForStateOrContext(ctx context.Context, state State) (State, error) {
	go func() {
		// Wake up when the context ends.
		defer rpty.cond.Broadcast()
		<-ctx.Done()
	}()
	rpty.cond.L.Lock()
	defer rpty.cond.L.Unlock()
	for ctx.Err() == nil && state > rpty.state {
		rpty.cond.Wait()
	}
	if ctx.Err() != nil {
		return rpty.state, ctx.Err()
	}
	return rpty.state, rpty.error
}
