package ws

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"caiyun/internal/repository"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Exercise the real replay/query path without requiring the CGO SQLite driver.
type replayQueryConnector struct{ afterQuery func() }
type replayQueryDriver struct{ afterQuery func() }
type replayQueryConn struct{ afterQuery func() }
type emptyReplayRows struct{}

func (c replayQueryConnector) Connect(context.Context) (driver.Conn, error) {
	return replayQueryConn{c.afterQuery}, nil
}
func (c replayQueryConnector) Driver() driver.Driver { return replayQueryDriver{c.afterQuery} }
func (d replayQueryDriver) Open(string) (driver.Conn, error) {
	return replayQueryConn{d.afterQuery}, nil
}
func (replayQueryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected statement")
}
func (replayQueryConn) Close() error              { return nil }
func (replayQueryConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c replayQueryConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	if c.afterQuery != nil {
		c.afterQuery()
	}
	return emptyReplayRows{}, nil
}
func (emptyReplayRows) Columns() []string         { return []string{"id"} }
func (emptyReplayRows) Close() error              { return nil }
func (emptyReplayRows) Next([]driver.Value) error { return io.EOF }

type replayFlushWriter struct {
	*httptest.ResponseRecorder
	afterFlush func()
}

func (w replayFlushWriter) Flush() {
	w.ResponseRecorder.Flush()
	if w.afterFlush != nil {
		w.afterFlush()
	}
}

func TestSSEReplayRechecksSessionForBufferedLiveMessages(t *testing.T) {
	for _, scenario := range []string{"revoked during history query", "revoked between live writes", "valid session"} {
		t.Run(scenario, func(t *testing.T) {
			valid := true
			dbConn := sql.OpenDB(replayQueryConnector{afterQuery: func() {
				if scenario == "revoked during history query" {
					valid = false
				}
			}})
			defer dbConn.Close()
			db, err := gorm.Open(mysql.New(mysql.Config{Conn: dbConn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			authorization := &SessionAuthorization{ExpiresAt: time.Now().Add(time.Minute), Validate: func(context.Context) bool { return valid }}
			if !authorization.valid(context.Background()) {
				t.Fatal("handshake must pass")
			}
			client := &SSEClient{userID: 42, send: make(chan Message, 2)}
			client.send <- Message{Sequence: 2, UserID: 42, Type: "notification", Data: "second-private"}
			client.send <- Message{Sequence: 1, UserID: 42, Type: "notification", Data: "first-private"}
			recorder := httptest.NewRecorder()
			writer := replayFlushWriter{recorder, func() {
				if scenario == "revoked between live writes" {
					valid = false
				}
			}}
			ok := (&Hub{}).replaySSE(writer, httptest.NewRequest("GET", "/events", nil), 42, repository.NewWSMessageRepository(db), authorization, client)
			count := strings.Count(recorder.Body.String(), "data:")
			want := map[string]int{"revoked during history query": 0, "revoked between live writes": 1, "valid session": 2}[scenario]
			if count != want || ok != (scenario == "valid session") {
				t.Fatalf("replay returned=%t messages=%d, want %d: %s", ok, count, want, recorder.Body.String())
			}
		})
	}
}
