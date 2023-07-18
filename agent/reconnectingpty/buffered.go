package reconnectingpty

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/armon/circbuf"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/pty"
)

// bufferedBackend provides a reconnectable PTY by using a ring buffer to store
// scrollback.
type bufferedBackend struct {
	command *pty.Cmd

	activeConnsMutex sync.Mutex
	activeConns      map[string]net.Conn

	circularBuffer      *circbuf.Buffer
	circularBufferMutex sync.RWMutex

	ptty    pty.PTYCmd
	process pty.Process

	logger  slog.Logger
	metrics *prometheus.CounterVec

	// closeSession is used to close the session when the process dies.
	closeSession func(reason string)
}

// start initializes the backend and starts the pty.  It must be called only
// once.  If the context ends the process will be killed.
func (b *bufferedBackend) start(ctx context.Context) error {
	b.activeConns = map[string]net.Conn{}

	// Default to buffer 64KiB.
	circularBuffer, err := circbuf.NewBuffer(64 << 10)
	if err != nil {
		return xerrors.Errorf("create circular buffer: %w", err)
	}
	b.circularBuffer = circularBuffer

	// pty.Cmd duplicates Path as the first argument so remove it.
	cmd := pty.CommandContext(ctx, b.command.Path, b.command.Args[1:]...)
	cmd.Env = append(b.command.Env, "TERM=xterm-256color")
	cmd.Dir = b.command.Dir
	ptty, process, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	b.ptty = ptty
	b.process = process

	// Multiplex the output onto the circular buffer and each active connection.
	//
	// We do not need to separately monitor for the process exiting.  When it
	// exits, our ptty.OutputReader() will return EOF after reading all process
	// output.
	go func() {
		buffer := make([]byte, 1024)
		for {
			read, err := ptty.OutputReader().Read(buffer)
			if err != nil {
				// When the PTY is closed, this is triggered.
				// Error is typically a benign EOF, so only log for debugging.
				if errors.Is(err, io.EOF) {
					b.logger.Debug(ctx, "unable to read pty output; command might have exited", slog.Error(err))
				} else {
					b.logger.Warn(ctx, "unable to read pty output; command might have exited", slog.Error(err))
					b.metrics.WithLabelValues("output_reader").Add(1)
				}
				// Could have been killed externally or failed to start at all (command
				// not found for example).
				b.closeSession("unable to read pty output; command might have exited")
				break
			}
			part := buffer[:read]
			b.circularBufferMutex.Lock()
			_, err = b.circularBuffer.Write(part)
			b.circularBufferMutex.Unlock()
			if err != nil {
				b.logger.Error(ctx, "write to circular buffer", slog.Error(err))
				b.metrics.WithLabelValues("write_buffer").Add(1)
			}
			b.activeConnsMutex.Lock()
			for cid, conn := range b.activeConns {
				_, err = conn.Write(part)
				if err != nil {
					b.logger.Warn(ctx,
						"error writing to active conn",
						slog.F("other_conn_id", cid),
						slog.Error(err),
					)
					b.metrics.WithLabelValues("write").Add(1)
				}
			}
			b.activeConnsMutex.Unlock()
		}
	}()

	return nil
}

// attach attaches to the pty and replays the buffer.  If the context closes it
// will detach the connection but leave the process up.  A connection ID is
// required so that logs in the pty goroutine can reference the same ID
// reference in logs output by each individual connection when acting on those
// connections.
func (b *bufferedBackend) attach(ctx context.Context, connID string, conn net.Conn, height, width uint16) (pty.PTYCmd, error) {
	// Resize the PTY to initial height + width.
	err := b.ptty.Resize(height, width)
	if err != nil {
		// We can continue after this, it's not fatal!
		b.logger.Error(ctx, "reconnecting PTY initial resize failed, but will continue", slog.Error(err))
		b.metrics.WithLabelValues("resize").Add(1)
	}

	// Write any previously stored data for the TTY.
	b.circularBufferMutex.RLock()
	prevBuf := slices.Clone(b.circularBuffer.Bytes())
	b.circularBufferMutex.RUnlock()

	// Note that there is a small race here between writing buffered
	// data and storing conn in activeConns. This is likely a very minor
	// edge case, but we should look into ways to avoid it. Holding
	// activeConnsMutex would be one option, but holding this mutex
	// while also holding circularBufferMutex seems dangerous.
	_, err = conn.Write(prevBuf)
	if err != nil {
		b.metrics.WithLabelValues("write").Add(1)
		return nil, xerrors.Errorf("write buffer to conn: %w", err)
	}

	b.activeConnsMutex.Lock()
	b.activeConns[connID] = conn
	b.activeConnsMutex.Unlock()

	// Remove the connection once it closes.
	go func() {
		<-ctx.Done()
		b.activeConnsMutex.Lock()
		delete(b.activeConns, connID)
		b.activeConnsMutex.Unlock()
	}()

	return b.ptty, nil
}

// close closes all connections to the reconnecting PTY, clears the circular
// buffer, and kills the process.
func (b *bufferedBackend) close() error {
	b.activeConnsMutex.Lock()
	var err error
	for _, conn := range b.activeConns {
		err = errors.Join(conn.Close())
	}
	b.activeConnsMutex.Unlock()
	_ = b.ptty.Close()
	b.circularBufferMutex.Lock()
	b.circularBuffer.Reset()
	b.circularBufferMutex.Unlock()
	_ = b.process.Kill()
	return err
}
