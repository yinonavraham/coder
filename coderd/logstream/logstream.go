package logstream

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"

	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

type PubsubMessage struct {
	CreatedAfter int64 `json:"created_after"`
	EndOfLogs    bool  `json:"end_of_logs,omitempty"`
}

type Options[T any, V any] struct {
	NotifyChannel string
	Pubsub        database.Pubsub
	Logger        slog.Logger

	HasCompleted func(ctx context.Context) (bool, error)
	FetchLogs    func(ctx context.Context, after int64) ([]T, error)
	GetLogID     func(T) int64
	ConvertLogs  func([]T) []V
}

// ServeHTTP serves a stream of logs that occur after the "after" ID provided.
// It's expected that clients will first perform a request without "follow" and
// then subsequently open a WebSocket following additional logs. This ensures the
// UI doesn't flicker a bunch with new log updates, and also ensures that the
// client gets all the logs.
func ServeHTTP[T any, V any](rw http.ResponseWriter, r *http.Request, opts Options[T, V]) {
	var (
		ctx      = r.Context()
		actor, _ = dbauthz.ActorFromContext(ctx)
		follow   = r.URL.Query().Has("follow")
		afterRaw = r.URL.Query().Get("after")
	)

	var after int64
	// Only fetch logs created after the time provided.
	if afterRaw != "" {
		var err error
		after, err = strconv.ParseInt(afterRaw, 10, 64)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query param \"after\" must be an integer.",
				Validations: []codersdk.ValidationError{
					{Field: "after", Detail: "Must be an integer"},
				},
			})
			return
		}
	}

	// If we aren't following the logs, we end early and return the logs.
	if !follow {
		logs, err := opts.FetchLogs(ctx, after)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching provisioner logs.",
				Detail:  err.Error(),
			})
			return
		}
		if logs == nil {
			logs = []T{}
		}
		httpapi.Write(ctx, rw, http.StatusOK, opts.ConvertLogs(logs))
		return
	}

	// When following logs, it's important that we begin to listen for log updates immediately,
	// otherwise it's possible that we'll miss logs that occur between the time we fetch the logs
	// and the time we begin listening for updates.
	var (
		bufferedLogs  = make(chan []T, 128)
		endOfLogs     atomic.Bool
		lastSentLogID atomic.Int64
	)

	sendLogs := func(logs []T) {
		if len(logs) == 0 {
			return
		}
		select {
		case bufferedLogs <- logs:
			lastSentLogID.Store(opts.GetLogID(logs[len(logs)-1]))
		default:
			opts.Logger.Warn(ctx, "logs overflowing channel")
		}
	}

	closeSubscribe, err := opts.Pubsub.Subscribe(
		opts.NotifyChannel,
		func(ctx context.Context, message []byte) {
			if endOfLogs.Load() {
				return
			}
			pm := PubsubMessage{}
			err := json.Unmarshal(message, &pm)
			if err != nil {
				opts.Logger.Warn(ctx, "invalid logs notify message", slog.Error(err))
				return
			}

			if pm.CreatedAfter != 0 {
				logs, err := opts.FetchLogs(dbauthz.As(ctx, actor), pm.CreatedAfter)
				if err != nil {
					opts.Logger.Warn(ctx, "failed to get logs after", slog.Error(err))
					return
				}
				sendLogs(logs)
			}

			if pm.EndOfLogs {
				endOfLogs.Store(true)
				logs, err := opts.FetchLogs(dbauthz.As(ctx, actor), lastSentLogID.Load())
				if err != nil {
					opts.Logger.Warn(ctx, "get logs after", slog.Error(err))
					return
				}
				sendLogs(logs)
				bufferedLogs <- nil
			}
		},
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to subscribe to logs.",
			Detail:  err.Error(),
		})
		return
	}
	defer closeSubscribe()

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	go httpapi.Heartbeat(ctx, conn)

	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close() // Also closes conn.

	logs, err := opts.FetchLogs(ctx, after)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner logs.",
			Detail:  err.Error(),
		})
		return
	}
	if logs == nil {
		logs = []T{}
	}

	// The Go stdlib JSON encoder appends a newline character after message write.
	encoder := json.NewEncoder(wsNetConn)
	if len(logs) > 0 {
		err = encoder.Encode(opts.ConvertLogs(logs))
		if err != nil {
			return
		}
	}
	fmt.Printf("WROTE LOGS %d\n", len(logs))

	complete, err := opts.HasCompleted(ctx)
	if err != nil {
		opts.Logger.Warn(ctx, "check if logs are complete", slog.Error(err))
		return
	}
	if complete {
		// The logs are complete, so we can close the connection.
		return
	}

	for {
		select {
		case <-ctx.Done():
			opts.Logger.Debug(context.Background(), "logs context canceled")
			return
		case logs, ok := <-bufferedLogs:
			// A nil log is sent when complete!
			if !ok || logs == nil {
				opts.Logger.Debug(context.Background(), "reached the end of published logs")
				return
			}
			err = encoder.Encode(opts.ConvertLogs(logs))
			if err != nil {
				return
			}
		}
	}
}

func Client[T any](ctx context.Context, ws *websocket.Conn) (<-chan []T, func(), error) {
	ctx, wsNetConn := websocketNetConn(ctx, ws, websocket.MessageText)
	decoder := json.NewDecoder(wsNetConn)
	closed := make(chan struct{})
	logChunks := make(chan []T)
	go func() {
		defer close(closed)
		defer close(logChunks)
		for {
			var logs []T
			err := decoder.Decode(&logs)
			if err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case logChunks <- logs:
			}
		}
	}()
	return logChunks, func() {
		_ = wsNetConn.Close()
		<-closed
	}, nil
}

// wsNetConn wraps net.Conn created by websocket.NetConn(). Cancel func
// is called if a read or write error is encountered.
// @typescript-ignore wsNetConn
type wsNetConn struct {
	cancel context.CancelFunc
	net.Conn
}

func (c *wsNetConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Close() error {
	defer c.cancel()
	return c.Conn.Close()
}

// websocketNetConn wraps websocket.NetConn and returns a context that
// is tied to the parent context and the lifetime of the conn. Any error
// during read or write will cancel the context, but not close the
// conn. Close should be called to release context resources.
func websocketNetConn(ctx context.Context, conn *websocket.Conn, msgType websocket.MessageType) (context.Context, net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	nc := websocket.NetConn(ctx, conn, msgType)
	return ctx, &wsNetConn{
		cancel: cancel,
		Conn:   nc,
	}
}
