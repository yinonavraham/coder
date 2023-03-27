package logstream_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/logstream"
)

func TestInvalidAfter(t *testing.T) {
	t.Parallel()
	rw := httptest.NewRecorder()
	logstream.ServeHTTP(rw, httptest.NewRequest("GET", "/?after=bananas", nil), logstream.Options[any, any]{})
	require.Equal(t, http.StatusBadRequest, rw.Code)
}

func TestNoLogs(t *testing.T) {
	t.Parallel()
	rw := httptest.NewRecorder()
	logstream.ServeHTTP(rw, httptest.NewRequest("GET", "/", nil), logstream.Options[any, any]{
		FetchLogs: func(ctx context.Context, after int64) ([]any, error) {
			return []any{}, nil
		},
		ConvertLogs: func(t []any) []any {
			return t
		},
	})
	require.Equal(t, http.StatusOK, rw.Code)
}

func TestFollow_NotifyEndBeforeFetch(t *testing.T) {
	// This test ensures that if notify is published before log fetching has
	// completed then the client will still receive the notification to end logs.
	t.Parallel()
	pubsub := database.NewPubsubInMemory()
	notifyChannel := "test"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logstream.ServeHTTP(w, r, logstream.Options[any, any]{
			Pubsub:        pubsub,
			NotifyChannel: notifyChannel,
			Logger:        slogtest.Make(t, nil),
			GetLogID: func(t any) int64 {
				return 0
			},
			FetchLogs: func(ctx context.Context, after int64) ([]any, error) {
				msg, err := json.Marshal(logstream.PubsubMessage{EndOfLogs: true})
				require.NoError(t, err)
				err = pubsub.Publish(notifyChannel, msg)
				require.NoError(t, err)
				return []any{}, nil
			},
			ConvertLogs: func(t []any) []any {
				return t
			},
			HasCompleted: func(_ context.Context) (bool, error) {
				return false, nil
			},
		})
	}))
	// nolint:bodyclose
	conn, _, err := websocket.Dial(context.Background(), srv.URL+"?follow", &websocket.DialOptions{})
	require.NoError(t, err)

	logs, closeFunc, err := logstream.Client[any](context.Background(), conn)
	require.NoError(t, err)
	defer closeFunc()
	<-logs
}

func TestFollow_StreamLogs(t *testing.T) {
	// This test ensures that if notify is published before log fetching has
	// completed then the client will still receive the notification to end logs.
	t.Parallel()
	pubsub := database.NewPubsubInMemory()
	notifyChannel := "test"

	totalLogs := make([]testLog, 0, 100)
	for i := 0; i < 100; i++ {
		totalLogs = append(totalLogs, testLog{ID: int64(i), Message: strconv.Itoa(i)})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logstream.ServeHTTP(w, r, logstream.Options[testLog, testLog]{
			Pubsub:        pubsub,
			NotifyChannel: notifyChannel,
			Logger:        slogtest.Make(t, nil).Leveled(slog.LevelDebug),
			GetLogID: func(t testLog) int64 {
				return t.ID
			},
			FetchLogs: func(ctx context.Context, after int64) ([]testLog, error) {
				fmt.Printf("Fetched logs! %d\n", after)
				return totalLogs[after:], nil
			},
			ConvertLogs: func(t []testLog) []testLog {
				return t[:]
			},
			HasCompleted: func(ctx context.Context) (bool, error) {
				return false, nil
			},
		})
	}))
	// nolint:bodyclose
	conn, _, err := websocket.Dial(context.Background(), srv.URL+"?follow", &websocket.DialOptions{})
	require.NoError(t, err)

	logs, closeFunc, err := logstream.Client[testLog](context.Background(), conn)
	require.NoError(t, err)
	defer closeFunc()
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		require.Len(t, log, len(totalLogs))

		// After the first publish, we should send an end of logs!
		data, _ := json.Marshal(logstream.PubsubMessage{EndOfLogs: true})
		pubsub.Publish(notifyChannel, data)
	}
}

type testLog struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}
