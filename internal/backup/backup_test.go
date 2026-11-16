package backup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/notify"
	"github.com/reefshepherd/reefshepherd/internal/scheduler"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

// fakeRaiser records raised events and, like the real service, reports false for an occurrence it has seen.
type fakeRaiser struct {
	events []notify.Event
	err    error
	seen   map[string]bool
}

func (f *fakeRaiser) Raise(_ context.Context, e notify.Event) (bool, error) {
	f.events = append(f.events, e)
	if f.err != nil {
		return false, f.err
	}
	key := fmt.Sprintf("%s/%s/%d/%s", e.Type, e.SourceKind, e.SourceID, e.Occurrence)
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	if f.seen[key] {
		return false, nil
	}
	f.seen[key] = true
	return true, nil
}

type env struct {
	t      *testing.T
	db     *db.DB
	store  *settings.Store
	dir    string
	parent string
	raiser *fakeRaiser
	deps   Deps
	svc    *Service
	mux    *http.ServeMux
}

func newEnv(t *testing.T) *env {
	t.Helper()
	database := testutil.OpenDB(t)
	parent := t.TempDir()
	e := &env{t: t, db: database, store: settings.NewStore(database), parent: parent, dir: filepath.Join(parent, "backups"), raiser: &fakeRaiser{}}
	e.deps = Deps{DB: database, Settings: e.store, Logger: testutil.Logger(t), Dir: e.dir, Notify: e.raiser}
	e.svc = NewService(e.deps)
	e.mux = http.NewServeMux()
	Routes(e.mux, e.deps)
	return e
}

func (e *env) at(now time.Time) *env {
	e.svc.now = func() time.Time { return now }
	return e
}

func (e *env) do(role, method, path string) *httptest.ResponseRecorder {
	e.t.Helper()
	var opts []testutil.ReqOpt
	if role != "" {
		opts = append(opts, testutil.AsUser(1, role))
	}
	return testutil.DoJSON(e.t, e.mux, method, path, nil, opts...)
}

func (e *env) write(name, content string) string {
	e.t.Helper()
	if err := os.MkdirAll(e.dir, 0o750); err != nil {
		e.t.Fatal(err)
	}
	path := filepath.Join(e.dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
	return path
}

func (e *env) names(kind string) []string {
	e.t.Helper()
	all, err := e.svc.List()
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, b := range all {
		if kind == "" || b.Kind == kind {
			out = append(out, b.Name)
		}
	}
	return out
}

func (e *env) files() []string {
	e.t.Helper()
	entries, err := os.ReadDir(e.dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		e.t.Fatal(err)
	}
	var out []string
	for _, f := range entries {
		out = append(out, f.Name())
	}
	return out
}

func utc(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

func TestListShowsOnlyBackupNamesNewestFirst(t *testing.T) {
	e := newEnv(t)
	e.write("reefshepherd-20261101T033000Z.db", "aaa")
	e.write("reefshepherd-20261103T033000Z.db", "bbbbb")
	e.write("reefshepherd-premigrate-00002-20261102T120000Z.db", "c")
	e.write("reefshepherd-prerestore-20261104T010203Z.db", "dd")
	for _, name := range []string{"notes.txt", "reefshepherd-backup.db", "reefshepherd-20261101T033000Z.db.bak", "reefshepherd-20261301T033000Z.db", "reefshepherd-premigrate-2-20261102T120000Z.db", "REEFSHEPHERD-20261101T033000Z.db"} {
		e.write(name, "x")
	}
	if err := os.Mkdir(filepath.Join(e.dir, "reefshepherd-20261105T033000Z.db"), 0o750); err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []Backup{
		{Name: "reefshepherd-prerestore-20261104T010203Z.db", Kind: KindPrerestore, SizeBytes: 2, CreatedAt: "2026-11-04T01:02:03Z"},
		{Name: "reefshepherd-20261103T033000Z.db", Kind: KindScheduled, SizeBytes: 5, CreatedAt: "2026-11-03T03:30:00Z"},
		{Name: "reefshepherd-premigrate-00002-20261102T120000Z.db", Kind: KindPremigrate, SizeBytes: 1, CreatedAt: "2026-11-02T12:00:00Z"},
		{Name: "reefshepherd-20261101T033000Z.db", Kind: KindScheduled, SizeBytes: 3, CreatedAt: "2026-11-01T03:30:00Z"},
	}
	if len(got) != len(want) {
		t.Fatalf("listed %d backups %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		g := got[i]
		g.at = time.Time{}
		if g != want[i] {
			t.Errorf("backup %d = %+v, want %+v", i, g, want[i])
		}
	}
}

func TestListOfAMissingFolderIsEmpty(t *testing.T) {
	e := newEnv(t)
	rec := e.do("admin", http.MethodGet, "/api/v1/backups")
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if items := testutil.DecodeItems[Backup](t, rec); len(items) != 0 {
		t.Errorf("items = %v", items)
	}
}

func TestCreateWritesARestorableBackupBesideItsTemporaryName(t *testing.T) {
	e := newEnv(t).at(utc(2026, 11, 15, 10, 0))
	testutil.Exec(t, e.db, `INSERT INTO tanks (name) VALUES ('Display')`)
	// A leftover temporary file of the same name sits where the backup writes its own, so it only disappears if the write happens there.
	e.write("reefshepherd-20261115T100000Z.db.tmp", "stale")

	b, err := e.svc.Create(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "reefshepherd-20261115T100000Z.db" || b.Kind != KindScheduled || b.CreatedAt != "2026-11-15T10:00:00Z" || b.SizeBytes <= 0 {
		t.Errorf("backup = %+v", b)
	}
	if files := e.files(); !slices.Equal(files, []string{b.Name}) {
		t.Errorf("files = %v, want only the backup (no temporary file)", files)
	}
	if _, err := db.CheckFile(t.Context(), filepath.Join(e.dir, b.Name)); err != nil {
		t.Errorf("the backup does not pass the restore check: %v", err)
	}
	copyDB, err := db.Open(filepath.Join(e.dir, b.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	if n := testutil.Count(t, copyDB, `SELECT COUNT(*) FROM tanks WHERE name = 'Display'`); n != 1 {
		t.Errorf("tanks in the backup = %d, want 1", n)
	}
}

func TestPruneKeepsTheConfiguredCountPerKind(t *testing.T) {
	e := newEnv(t).at(utc(2026, 11, 15, 10, 0))
	for day := 1; day <= 20; day++ {
		e.write(fmt.Sprintf("reefshepherd-202610%02dT033000Z.db", day), "s")
	}
	for day := 1; day <= 8; day++ {
		e.write(fmt.Sprintf("reefshepherd-premigrate-00002-202608%02dT033000Z.db", day), "p")
		e.write(fmt.Sprintf("reefshepherd-prerestore-202609%02dT033000Z.db", day), "r")
	}
	for _, name := range []string{"notes.txt", "reefshepherd-backup.db", "reefshepherd-20261001T033000Z.db.bak"} {
		e.write(name, "keep me")
	}
	if err := e.store.SetBackupKeep(t.Context(), 3); err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	wantScheduled := []string{"reefshepherd-20261115T100000Z.db", "reefshepherd-20261020T033000Z.db", "reefshepherd-20261019T033000Z.db"}
	if got := e.names(KindScheduled); !slices.Equal(got, wantScheduled) {
		t.Errorf("scheduled = %v, want %v", got, wantScheduled)
	}
	if got := e.names(KindPremigrate); len(got) != 5 || got[0] != "reefshepherd-premigrate-00002-20260808T033000Z.db" || got[4] != "reefshepherd-premigrate-00002-20260804T033000Z.db" {
		t.Errorf("premigrate = %v, want the newest five", got)
	}
	if got := e.names(KindPrerestore); len(got) != 5 || got[0] != "reefshepherd-prerestore-20260908T033000Z.db" || got[4] != "reefshepherd-prerestore-20260904T033000Z.db" {
		t.Errorf("prerestore = %v, want the newest five", got)
	}
	for _, name := range []string{"notes.txt", "reefshepherd-backup.db", "reefshepherd-20261001T033000Z.db.bak"} {
		if _, err := os.Stat(filepath.Join(e.dir, name)); err != nil {
			t.Errorf("%s was touched by the prune: %v", name, err)
		}
	}
}

func TestPruneSweepsOnlyOldTemporaryBackupFiles(t *testing.T) {
	e := newEnv(t).at(utc(2026, 11, 15, 10, 0))
	old := e.write("reefshepherd-20261114T033000Z.db.tmp", "old")
	recent := e.write("reefshepherd-20261115T093000Z.db.tmp", "recent")
	foreign := e.write("download.tmp", "someone else's")
	for path, age := range map[string]time.Duration{old: 90 * time.Minute, recent: 10 * time.Minute, foreign: 5 * time.Hour} {
		mtime := e.svc.now().Add(-age)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := e.svc.Create(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(old); err == nil {
		t.Error("a temporary file older than an hour was kept")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("a recent temporary file was removed: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("a file that is not a backup temporary was removed: %v", err)
	}
}

func TestCreateInTheSameSecondIsAConflict(t *testing.T) {
	e := newEnv(t).at(utc(2026, 11, 15, 10, 0))
	testutil.ExpectStatus(t, e.do("admin", http.MethodPost, "/api/v1/backups"), http.StatusCreated)
	testutil.ExpectError(t, e.do("admin", http.MethodPost, "/api/v1/backups"), http.StatusConflict, "conflict")
	if got := e.names(""); len(got) != 1 {
		t.Errorf("backups = %v, want one", got)
	}
}

// jobFor returns the backup job built on e's dependencies, with its service clock fixed.
func (e *env) job() scheduler.Job {
	e.t.Helper()
	jobs := Jobs(e.deps)
	if len(jobs) != 1 || jobs[0].Name != "backup" {
		e.t.Fatalf("jobs = %+v", jobs)
	}
	return jobs[0]
}

func (e *env) setZoneAndTime(zone, hhmm string) {
	e.t.Helper()
	if err := e.store.SetTimezone(e.t.Context(), zone); err != nil {
		e.t.Fatal(err)
	}
	if err := e.store.SetBackupTime(e.t.Context(), hhmm); err != nil {
		e.t.Fatal(err)
	}
}

func TestJobTriggerFollowsTheBackupTimeInTheInstanceTimezone(t *testing.T) {
	e := newEnv(t)
	e.setZoneAndTime("America/New_York", "03:30")
	trigger := e.job().Trigger
	// 03:30 in New York on 15 November is 08:30 UTC.
	due := func(now time.Time, last time.Time) bool {
		t.Helper()
		ok, err := trigger.Due(t.Context(), now, last)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if due(utc(2026, 11, 15, 8, 29), time.Time{}) {
		t.Error("due a minute before 03:30 local")
	}
	if !due(utc(2026, 11, 15, 8, 30), time.Time{}) {
		t.Error("not due at 03:30 local")
	}

	if err := e.store.SetBackupTime(t.Context(), "05:00"); err != nil {
		t.Fatal(err)
	}
	if due(utc(2026, 11, 15, 8, 30), time.Time{}) {
		t.Error("still due at 03:30 local after the time was moved to 05:00")
	}
	if !due(utc(2026, 11, 15, 10, 0), time.Time{}) {
		t.Error("not due at 05:00 local")
	}
}

func TestJobBacksUpAtTheSlotAndCatchesUpAfterDowntime(t *testing.T) {
	e := newEnv(t)
	e.setZoneAndTime("America/New_York", "03:30")
	job := e.job()
	catchUp := utc(2026, 11, 15, 14, 7)

	if ok, err := job.Trigger.Due(t.Context(), catchUp, utc(2026, 11, 14, 8, 30)); err != nil || !ok {
		t.Fatalf("not due for a catch-up after downtime: %v, %v", ok, err)
	}
	if err := job.Run(t.Context(), catchUp); err != nil {
		t.Fatal(err)
	}
	if got := e.names(KindScheduled); !slices.Equal(got, []string{"reefshepherd-20261115T140700Z.db"}) {
		t.Errorf("backups = %v", got)
	}
	if ok, _ := job.Trigger.Due(t.Context(), catchUp.Add(time.Minute), catchUp); ok {
		t.Error("due again on the same local day")
	}
	if len(e.raiser.events) != 0 {
		t.Errorf("a successful backup raised %v", e.raiser.events)
	}
}

func TestJobDoesNotBackUpTwiceAfterARestart(t *testing.T) {
	e := newEnv(t)
	e.setZoneAndTime("America/New_York", "03:30")
	// Yesterday's backup is before today's slot and must not count.
	e.write("reefshepherd-20261114T083000Z.db", "yesterday")

	first := utc(2026, 11, 15, 8, 30)
	if err := e.job().Run(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if got := e.names(KindScheduled); len(got) != 2 {
		t.Fatalf("backups after the first run = %v, want yesterday's and today's", got)
	}

	// A restart builds new jobs whose trigger has never fired, so it fires again; the run itself must see today's file.
	restarted := Jobs(e.deps)[0]
	if ok, _ := restarted.Trigger.Due(t.Context(), utc(2026, 11, 15, 8, 31), time.Time{}); !ok {
		t.Fatal("a fresh trigger should fire after a restart")
	}
	if err := restarted.Run(t.Context(), utc(2026, 11, 15, 8, 31)); err != nil {
		t.Fatal(err)
	}
	if got := e.names(KindScheduled); len(got) != 2 {
		t.Errorf("backups after the restart = %v, want still two", got)
	}
}

func TestJobCountsAManualBackupMadeSinceTheSlot(t *testing.T) {
	e := newEnv(t).at(utc(2026, 11, 15, 9, 0))
	e.setZoneAndTime("America/New_York", "03:30")
	if _, err := e.svc.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := e.job().Run(t.Context(), utc(2026, 11, 15, 9, 5)); err != nil {
		t.Fatal(err)
	}
	if got := e.names(KindScheduled); len(got) != 1 {
		t.Errorf("backups = %v, want only the manual one", got)
	}
}

func TestJobDoesNothingWhenBackupsAreOff(t *testing.T) {
	e := newEnv(t)
	if err := e.store.SetBackupEnabled(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if err := e.job().Run(t.Context(), utc(2026, 11, 15, 12, 0)); err != nil {
		t.Fatal(err)
	}
	if got := e.files(); len(got) != 0 {
		t.Errorf("files = %v, want none", got)
	}
}

func TestFailedBackupRaisesOneCriticalNotificationPerSlot(t *testing.T) {
	e := newEnv(t)
	e.setZoneAndTime("America/New_York", "03:30")
	// A regular file where the folder should be makes every backup fail.
	blocker := filepath.Join(e.parent, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e.deps.Dir = filepath.Join(blocker, "backups")
	job := Jobs(e.deps)[0]

	first := utc(2026, 11, 15, 8, 30)
	if err := job.Run(t.Context(), first); err == nil {
		t.Fatal("the failed backup did not return an error")
	}
	if len(e.raiser.events) != 1 {
		t.Fatalf("events = %+v, want one", e.raiser.events)
	}
	ev := e.raiser.events[0]
	if ev.Type != "backup_failed" || ev.Severity != "critical" || ev.Link != "/settings/backups" || ev.SourceKind != "backup" || ev.SourceID != 0 || ev.TankID != nil {
		t.Errorf("event = %+v", ev)
	}
	if ev.Occurrence != "2026-11-15T08:30:00Z" {
		t.Errorf("occurrence = %q, want the slot instant", ev.Occurrence)
	}
	if ev.Title == "" || !strings.Contains(ev.Body, "2026-11-15 03:30") || strings.Contains(ev.Body, "backup_failed") {
		t.Errorf("title %q body %q should be rendered text naming the local slot", ev.Title, ev.Body)
	}

	// A retry in the same slot raises the same occurrence, which the notification service drops.
	_ = job.Run(t.Context(), first.Add(time.Minute))
	if len(e.raiser.events) != 2 || e.raiser.events[1].Occurrence != ev.Occurrence {
		t.Errorf("retry events = %+v, want the same occurrence", e.raiser.events)
	}
	if len(e.raiser.seen) != 1 {
		t.Errorf("distinct occurrences = %d, want 1", len(e.raiser.seen))
	}
	// The next day is a new slot.
	_ = job.Run(t.Context(), first.Add(24*time.Hour))
	if len(e.raiser.seen) != 2 {
		t.Errorf("distinct occurrences after the next day = %d, want 2", len(e.raiser.seen))
	}
}

func TestFailureToRaiseIsLoggedNotReturned(t *testing.T) {
	e := newEnv(t)
	e.raiser.err = errors.New("notify is down")
	blocker := filepath.Join(e.parent, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e.deps.Dir = filepath.Join(blocker, "backups")
	err := Jobs(e.deps)[0].Run(t.Context(), utc(2026, 11, 15, 12, 0))
	if err == nil || strings.Contains(err.Error(), "notify is down") {
		t.Errorf("err = %v, want the backup error", err)
	}
}

func TestEveryBackupRouteIsAdminOnly(t *testing.T) {
	e := newEnv(t)
	name := "reefshepherd-20261101T033000Z.db"
	e.write(name, "data")
	routes := [][2]string{
		{http.MethodGet, "/api/v1/backups"},
		{http.MethodPost, "/api/v1/backups"},
		{http.MethodGet, "/api/v1/backups/" + name + "/download"},
		{http.MethodDelete, "/api/v1/backups/" + name},
	}
	for _, r := range routes {
		testutil.ExpectError(t, e.do("", r[0], r[1]), http.StatusUnauthorized, "unauthorized")
		testutil.ExpectError(t, e.do("member", r[0], r[1]), http.StatusForbidden, "forbidden")
	}
	if got := e.names(""); !slices.Equal(got, []string{name}) {
		t.Errorf("a refused request changed the backups: %v", got)
	}
}

func TestAPIListCreateDownloadDelete(t *testing.T) {
	e := newEnv(t)

	rec := e.do("admin", http.MethodPost, "/api/v1/backups")
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	created := testutil.Decode[Backup](t, rec)
	if kind, at, ok := parseName(created.Name); !ok || kind != KindScheduled || created.Kind != "scheduled" || created.CreatedAt != db.FormatTime(at) || created.SizeBytes == 0 {
		t.Errorf("created = %+v", created)
	}
	if at, err := db.ParseTime(created.CreatedAt); err != nil || time.Since(at) > time.Minute {
		t.Errorf("created_at = %q, want the current time (%v)", created.CreatedAt, err)
	}

	listed := testutil.DecodeItems[Backup](t, e.do("admin", http.MethodGet, "/api/v1/backups"))
	if len(listed) != 1 || listed[0].Name != created.Name || listed[0].SizeBytes != created.SizeBytes {
		t.Errorf("listed = %+v", listed)
	}

	dl := e.do("admin", http.MethodGet, "/api/v1/backups/"+created.Name+"/download")
	testutil.ExpectStatus(t, dl, http.StatusOK)
	if got := dl.Header().Get("Content-Type"); got != "application/vnd.sqlite3" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := dl.Header().Get("Content-Disposition"); got != `attachment; filename="`+created.Name+`"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if got := dl.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	onDisk, err := os.ReadFile(filepath.Join(e.dir, created.Name))
	if err != nil {
		t.Fatal(err)
	}
	if dl.Body.String() != string(onDisk) {
		t.Error("the download differs from the file")
	}

	testutil.ExpectStatus(t, e.do("admin", http.MethodDelete, "/api/v1/backups/"+created.Name), http.StatusNoContent)
	testutil.ExpectError(t, e.do("admin", http.MethodDelete, "/api/v1/backups/"+created.Name), http.StatusNotFound, "not_found")
	testutil.ExpectError(t, e.do("admin", http.MethodGet, "/api/v1/backups/"+created.Name+"/download"), http.StatusNotFound, "not_found")
}

func TestNamesThatAreNotBackupNamesAreNotFound(t *testing.T) {
	e := newEnv(t)
	valid := "reefshepherd-20261101T033000Z.db"
	e.write(valid, "data")
	e.write("notes.txt", "private notes")
	secret := filepath.Join(e.parent, "secret.db")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A symlink with a valid backup name must not lead outside the folder either.
	if err := os.Symlink(secret, filepath.Join(e.dir, "reefshepherd-20261102T033000Z.db")); err != nil {
		t.Fatal(err)
	}

	names := []string{
		"..%2Fsecret.db",
		"..%2F..%2Fetc%2Fpasswd",
		"%2e%2e",
		"%2e%2e%2Fsecret.db",
		"..%5Csecret.db",
		"notes.txt",
		valid + "%00",
		"%2Fetc%2Fpasswd",
		"reefshepherd-20261102T033000Z.db",
		"reefshepherd-20261199T033000Z.db",
		"REEFSHEPHERD-20261101T033000Z.db",
		valid + ".tmp",
	}
	for _, name := range names {
		for _, r := range [][2]string{
			{http.MethodGet, "/api/v1/backups/" + name + "/download"},
			{http.MethodDelete, "/api/v1/backups/" + name},
		} {
			rec := e.do("admin", r[0], r[1])
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404; body=%s", r[0], r[1], rec.Code, rec.Body)
			}
			if strings.Contains(rec.Body.String(), "top secret") || strings.Contains(rec.Body.String(), "private notes") {
				t.Errorf("%s %s leaked a file", r[0], r[1])
			}
		}
	}
	for _, path := range []string{secret, filepath.Join(e.dir, "notes.txt"), filepath.Join(e.dir, valid)} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was deleted: %v", path, err)
		}
	}
}
