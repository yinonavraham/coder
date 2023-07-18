package reconnectingpty

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/pty"
)

// screenBackend provides a reconnectable PTY via `screen`.
type screenBackend struct {
	command *pty.Cmd

	// id holds the id of the session for both creating and attaching.  This will
	// be generated uniquely for each session because without control of the
	// screen daemon we do not have its PID and without the PID screen will do
	// partial matching.  Enforcing a unique ID should guarantee we match on the
	// right session.
	id string

	// mutex prevents concurrent attaches to the session.  Screen will happily
	// spawn two separate sessions with the same name if multiple attaches happen
	// in a close enough interval.  We are not able to control the screen daemon
	// ourselves to prevent this because the daemon will spawn with a hardcoded
	// 24x80 size which results in confusing padding above the prompt once the
	// attach comes in and resizes.
	mutex sync.Mutex

	configFile string

	logger  slog.Logger
	metrics *prometheus.CounterVec
}

// start writes config settings and creates the socket directory.  It must only
// be called once. If we could, we would want to spawn the daemon here and
// attach each connection to it but since doing that spawns the daemon with a
// hardcoded 24x80 size it is not a very good user experience.  Instead we will
// let the attach command spawn the daemon on its own which causes it to spawn
// with the specified size.
func (b *screenBackend) start(ctx context.Context) error {
	// Socket paths are limited to around 100 characters on Linux and macOS which
	// depending on the temporary directory can be a problem.  To give more leeway
	// use a short ID.
	buf := make([]byte, 4)
	_, err := rand.Read(buf)
	if err != nil {
		return xerrors.Errorf("generate screen id: %w", err)
	}
	b.id = hex.EncodeToString(buf)

	settings := []string{
		// Tell screen not to handle motion for xterm* terminals which allows
		// scrolling the terminal via the mouse wheel or scroll bar (by default
		// screen uses it to cycle through the command history).  There does not
		// seem to be a way to make screen itself scroll on mouse wheel.  tmux can
		// do it but then there is no scroll bar and it kicks you into copy mode
		// where keys stop working until you exit copy mode which seems like it
		// could be confusing.
		"termcapinfo xterm* ti@:te@",
		// Enable alternate screen emulation otherwise applications get rendered in
		// the current window which wipes out visible output resulting in missing
		// output when scrolling back with the mouse wheel (copy mode still works
		// since that is screen itself scrolling).
		"altscreen on",
		// Remap the control key to C-s since C-a may be used in applications.  C-s
		// cannot actually be used anyway since by default it will pause and C-q to
		// resume will just kill the browser window.  We may not want people using
		// the control key anyway since it will not be obvious they are in screen
		// and doing things like switching windows makes mouse wheel scroll wonky
		// due to the terminal doing the scrolling rather than screen itself (but
		// again copy mode will work just fine).
		"escape ^Ss",
	}

	b.configFile = filepath.Join(os.TempDir(), "coder-screen", "config")
	err = os.MkdirAll(filepath.Dir(b.configFile), 0o700)
	if err != nil {
		return err
	}

	return os.WriteFile(b.configFile, []byte(strings.Join(settings, "\n")), 0o600)
}

// attach attaches to the screen session by ID (spawning it if necessary) and
// hooks up screen's output to the connection.  If the context ends it will kill
// the process for this connection (not the screen daemon), detaching it.
func (b *screenBackend) attach(ctx context.Context, _ string, conn net.Conn, height, width uint16) (pty.PTYCmd, error) {
	// Ensure another attach does not come in and spawn a duplicate session.
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// Wrap the command with screen and tie it to the connection's context.
	cmd := pty.CommandContext(ctx, "screen", append([]string{
		// -S is for setting the session's name.
		"-S", b.id,
		// -x allows attaching to an already attached session.
		// -RR reattaches to the daemon or creates the session daemon if missing.
		// -q disables the "New screen..." message that appears for five seconds
		//    when creating a new session with -RR.
		// -c is the flag for the config file.
		"-xRRqc", b.configFile,
		b.command.Path,
		// pty.Cmd duplicates Path as the first argument so remove it.
	}, b.command.Args[1:]...)...)
	cmd.Env = append(b.command.Env, "TERM=xterm-256color")
	cmd.Dir = b.command.Dir
	ptty, process, err := pty.Start(cmd, pty.WithPTYOption(
		pty.WithSSHRequest(ssh.Pty{
			Window: ssh.Window{
				// Make sure to spawn at the right size because if we resize afterward it
				// leaves confusing padding (screen will resize such that the screen
				// contents are aligned to the bottom).
				Height: int(height),
				Width:  int(width),
			},
		}),
	))
	if err != nil {
		b.metrics.WithLabelValues("screen_spawn").Add(1)
		return nil, err
	}

	b.logger.Debug(ctx, "screen spawned", slog.F("id", b.id))

	// Pipe the process's output to the connection.
	// We do not need to separately monitor for the process exiting.  When it
	// exits, our ptty.OutputReader() will return EOF after reading all process
	// output.
	go func() {
		defer conn.Close()
		buffer := make([]byte, 1024)
		for {
			read, err := ptty.OutputReader().Read(buffer)
			if err != nil {
				// When the PTY is closed, this is triggered.
				// Error is typically a benign EOF, so only log for debugging.
				if errors.Is(err, io.EOF) {
					b.logger.Debug(ctx, "unable to read pty output; screen might have exited", slog.Error(err))
				} else {
					b.logger.Warn(ctx, "unable to read pty output; screen might have exited", slog.Error(err))
					b.metrics.WithLabelValues("screen_output_reader").Add(1)
				}
				// The process might have died because the session itself died or it
				// might have been separately killed and the session is still up or the
				// connection might have been closed.
				break
			}
			part := buffer[:read]
			_, err = conn.Write(part)
			if err != nil {
				// Connection might have been closed.
				if errors.Unwrap(err).Error() != "endpoint is closed for send" {
					b.logger.Warn(ctx, "error writing to active conn", slog.Error(err))
					b.metrics.WithLabelValues("screen_write").Add(1)
				}
				break
			}
		}
	}()

	// Kill the process when the connection closes.
	go func() {
		<-ctx.Done()
		_ = process.Kill()
		_ = ptty.Close()
	}()

	// Version seems to be the only command without a side effect (other than
	// making the version pop up briefly) so use it to wait for the session to
	// come up.  If we do not wait we could end up spawning multiple sessions with
	// the same name.
	err = b.sendCommand(ctx, "version", nil)
	if err != nil {
		_ = process.Kill()
		_ = ptty.Close()
		b.metrics.WithLabelValues("screen_wait").Add(1)
		return nil, err
	}

	return ptty, nil
}

// close asks screen to kill the session by its ID.
func (b *screenBackend) close() error {
	// If the command errors that the session is already gone that is fine.
	return b.sendCommand(context.Background(), "quit", []string{"No screen session found"})
}

// sendCommand runs a screen command against a running screen session.  If the
// command fails with an error matching anything in successErrors it will be
// considered a success state (for example "no session" when quitting and the
// session is already dead).  The command will be retried until successful, the
// timeout is reached, or the context ends in which case the context error is
// returned together with the last error from the command.
func (b *screenBackend) sendCommand(ctx context.Context, command string, successErrors []string) error {
	ctx, cancel := context.WithTimeout(ctx, attachTimeout)
	defer cancel()

	var lastErr error
	attempts := 0
	run := func() bool {
		attempts = attempts + 1

		var stdout bytes.Buffer
		//nolint:gosec
		cmd := exec.CommandContext(ctx, "screen",
			// -S is for setting the session's name.
			"-S", b.id,
			// -c is the flag for the config file.
			"-c", b.configFile,
			// -X runs a command in the matching session.
			"-X", command,
		)
		cmd.Env = append(b.command.Env, "TERM=xterm-256color")
		cmd.Dir = b.command.Dir
		cmd.Stdout = &stdout
		lastErr = cmd.Run()
		if lastErr == nil {
			return true
		}

		stdoutStr := stdout.String()
		for _, se := range successErrors {
			if strings.Contains(stdoutStr, se) {
				return true
			}
		}

		// Things like "exit status 1" are imprecise so include stdout since it may
		// contain more information ("no screen session found" for example).
		lastErr = xerrors.Errorf("`screen -S %s -X %s`: %w: %s (attempt %d)", b.id, command, lastErr, stdoutStr, attempts)

		return false
	}

	// Run immediately.
	if done := run(); done {
		return nil
	}

	// Then run on an interval.
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-ticker.C:
			if done := run(); done {
				return nil
			}
		}
	}
}
