package dashboard

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/tanks"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// spyDriver wraps the SQLite driver and calls before with the text of every query a connection runs, so a test can commit a write between two reads.
type spyDriver struct {
	inner  driver.Driver
	dsn    string
	before func(query string)
}

func (d spyDriver) Connect(context.Context) (driver.Conn, error) {
	c, err := d.inner.Open(d.dsn)
	if err != nil {
		return nil, err
	}
	return spyConn{Conn: c, before: d.before}, nil
}

func (d spyDriver) Driver() driver.Driver { return d.inner }

type spyConn struct {
	driver.Conn
	before func(query string)
}

func (c spyConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

func (c spyConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.before(query)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

// spyEnv returns a service whose reader pool calls before with each query's text, and an env that writes through the ordinary pools of the same database.
func spyEnv(t *testing.T, before func(query string)) (*Service, *env) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reef.db")
	database := testutil.OpenDBAt(t, path)
	plain, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	inner := plain.Driver()
	_ = plain.Close()
	reader := sql.OpenDB(spyDriver{inner: inner, dsn: "file:" + path + "?_pragma=query_only(1)&_pragma=busy_timeout(5000)", before: before})
	t.Cleanup(func() { _ = reader.Close() })
	testutil.Exec(t, database, `INSERT INTO users (id, username) VALUES (?, 'alice')`, userA)
	svc := NewService(Deps{DB: &db.DB{W: database.W, R: reader}, Logger: testutil.Logger(t)})
	return svc, &env{t: t, db: database}
}

// onceOn runs fn the first time a query containing marker is about to run.
func onceOn(marker string, fn func()) func(string) {
	done := false
	return func(query string) {
		if !done && strings.Contains(query, marker) {
			done = true
			fn()
		}
	}
}

func TestGetReadsCardsAndParametersFromOneSnapshot(t *testing.T) {
	var e *env
	// The card list is the last read of Get; a write committed just before it must not show.
	svc, env := spyEnv(t, onceOn("ORDER BY position", func() {
		testutil.Exec(e.t, e.db, `UPDATE dashboard_cards SET size = 'wide'`)
	}))
	e = env
	tank := e.tank("Display")
	param := e.param("A")
	e.enable(tank, param, true, 0)
	// The first call seeds the layout through the writer, which the spy does not see.
	if _, err := svc.Get(t.Context(), userA, tank); err != nil {
		t.Fatal(err)
	}

	layout, err := svc.Get(t.Context(), userA, tank)
	if err != nil {
		t.Fatal(err)
	}
	wantShape(t, layout.Cards, "tests_due:-:normal", "maintenance_due:-:normal", "parameter:"+id(param)+":normal")

	// The write did commit, so the next read sees it.
	layout, err = svc.Get(t.Context(), userA, tank)
	if err != nil {
		t.Fatal(err)
	}
	wantShape(t, layout.Cards, "tests_due:-:wide", "maintenance_due:-:wide", "parameter:"+id(param)+":wide")
}

func TestGetSeedingRechecksTheTankInsideTheWrite(t *testing.T) {
	var e *env
	var tank int64
	// The tank disappears after the first read has seen it and before the seeding write starts.
	svc, env := spyEnv(t, onceOn("FROM dashboard_cards", func() {
		testutil.Exec(e.t, e.db, `DELETE FROM tanks WHERE id = ?`, tank)
	}))
	e = env
	tank = e.tank("Display")

	_, err := svc.Get(t.Context(), userA, tank)
	if !errors.Is(err, tanks.ErrNotFound) {
		t.Fatalf("err = %v, want tanks.ErrNotFound", err)
	}
}
