package app

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/reefshepherd/reefshepherd/internal/auth"
	"github.com/reefshepherd/reefshepherd/internal/auth/password"
	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
	"github.com/reefshepherd/reefshepherd/internal/settings"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
	"github.com/reefshepherd/reefshepherd/internal/webui"
)

// TestRegisterRoutesWiresRealDepsIntoPackages drives the route table directly, so a package left with empty dependencies shows up as a failure here.
func TestRegisterRoutesWiresRealDepsIntoPackages(t *testing.T) {
	database := openTestDB(t)
	store := settings.NewStore(database)
	if err := store.SetInstanceName(t.Context(), "Wired Reef"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataDir: "/srv/reef", PublicURL: "https://reef.example.org"}
	authSvc := auth.NewService(auth.Deps{
		DB: database, Settings: store, Passwords: password.NewHasher(password.Params{Time: 1, MemoryKiB: 64, Threads: 1}),
		Config: cfg, Logger: testutil.Logger(t),
	})
	deps := newRouteDeps(cfg, testutil.Logger(t), database, store, authSvc)
	requireWiredFeatureDeps(t, deps, database, store)
	mux := http.NewServeMux()
	registerRoutes(mux, deps, webui.NewHandler(fstest.MapFS{".gitkeep": {}}, testutil.Logger(t)))

	serve := func(method, target, body, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		rec := httptest.NewRecorder()
		// The same panic guard the real stack has, so a missing dependency shows up as a 500 here.
		httpx.Recover(testutil.Logger(t))(mux).ServeHTTP(rec, r)
		return rec
	}

	// auth reads the instance name from the store it was given.
	rec := serve(http.MethodGet, "/api/v1/auth/config", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"instance_name":"Wired Reef"`) || !strings.Contains(rec.Body.String(), `"setup_required":true`) {
		t.Errorf("auth config: %d %s", rec.Code, rec.Body)
	}

	// users writes through the database and hashes with the auth service's hasher.
	rec = serve(http.MethodPost, "/api/v1/setup", `{"username":"wired","password":"correct horse battery","instance_name":"Setup Named"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	var n int
	if err := database.R.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM users WHERE username = 'wired' AND password_hash LIKE '$argon2id$%'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("users with the new account = %d, %v", n, err)
	}
	var cookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("setup did not return a session")
	}

	// settings reads through the same store the other packages share and gets the sign-in check from auth.
	rec = serve(http.MethodGet, "/api/v1/settings", "", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"instance_name":"Setup Named"`) {
		t.Errorf("settings: %d %s", rec.Code, rec.Body)
	}
	rec = serve(http.MethodGet, "/api/v1/me", "", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"wired"`) {
		t.Errorf("me: %d %s", rec.Code, rec.Body)
	}
	// Without the cookie everything behind the sign-in check is closed, including paths no package serves.
	for _, target := range []string{"/api/v1/settings", "/api/v1/me", "/api/v1/tanks", "/api/v1/does-not-exist"} {
		if rec := serve(http.MethodGet, target, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session: %d", target, rec.Code)
		}
	}
	// Public pages and the unit catalog stay public.
	if rec := serve(http.MethodGet, "/api/v1/units", "", ""); rec.Code != http.StatusOK {
		t.Errorf("units without a session: %d", rec.Code)
	}
	if rec := serve(http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Errorf("healthz: %d", rec.Code)
	}
	if rec := serve(http.MethodGet, "/some/page", "", ""); rec.Code != http.StatusOK {
		t.Errorf("web ui: %d", rec.Code)
	}
}

// requireWiredFeatureDeps checks that each feature package got the shared database and settings, the paths and URLs derived from the config, and one common notification service.
func requireWiredFeatureDeps(t *testing.T, d routeDeps, database *db.DB, store *settings.Store) {
	t.Helper()
	if d.Notify.DB != database || d.Notify.Settings != store || d.Notify.Logger == nil || d.Notify.PublicURL != "https://reef.example.org" {
		t.Errorf("notify deps = %+v", d.Notify)
	}
	if d.Maintenance.DB != database || d.Maintenance.Settings != store || d.Maintenance.Logger == nil {
		t.Errorf("maintenance deps = %+v", d.Maintenance)
	}
	if d.Backup.DB != database || d.Backup.Settings != store || d.Backup.Logger == nil || d.Backup.Dir != "/srv/reef/backups" {
		t.Errorf("backup deps = %+v", d.Backup)
	}
	r := d.RemindersDeps
	if r.DB != database || r.Settings != store || r.Logger == nil || r.Maintenance == nil || r.Params == nil || d.Reminders == nil {
		t.Errorf("reminders deps = %+v", r)
	}
	if r.Notify == nil || r.Notify != d.Backup.Notify {
		t.Errorf("reminders and backup raise through different notification services: %v and %v", r.Notify, d.Backup.Notify)
	}
	if r.Params != nil && r.Params.OnResultCreated != nil {
		t.Error("the params service the reminders read from has the result hook, which would call the reminders back")
	}
}

var (
	apiRoutePattern = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) /api/`)
	pathWildcard    = regexp.MustCompile(`\{[a-z]+\}`)
)

// declaredAPIRoutes reads the feature packages' source for the method-and-path patterns they register under /api/, split by whether the registering function is PublicRoutes.
func declaredAPIRoutes(t *testing.T) (public, protected []string) {
	t.Helper()
	files, err := filepath.Glob("../*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				lit, isLit := call.Args[0].(*ast.BasicLit)
				if !ok || !isLit || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
					return true
				}
				pattern, err := strconv.Unquote(lit.Value)
				if err != nil || !apiRoutePattern.MatchString(pattern) {
					return true
				}
				if fn.Name.Name == "PublicRoutes" {
					public = append(public, pattern)
				} else {
					protected = append(protected, pattern)
				}
				return true
			})
		}
	}
	slices.Sort(public)
	slices.Sort(protected)
	return public, protected
}

// TestEveryNonPublicRouteRejectsAnonymousRequests finds each route a feature package registers and expects every one outside the short public list to answer 401 without a session, once setup is done.
func TestEveryNonPublicRouteRejectsAnonymousRequests(t *testing.T) {
	public, protected := declaredAPIRoutes(t)
	wantPublic := []string{
		"GET /api/v1/auth/config", "GET /api/v1/units", "POST /api/v1/auth/login",
		"POST /api/v1/auth/logout", "POST /api/v1/auth/mfa", "POST /api/v1/setup",
	}
	if !slices.Equal(public, wantPublic) {
		t.Fatalf("public routes = %v, want %v", public, wantPublic)
	}
	if !slices.Contains(protected, "GET /api/v1/tanks") {
		t.Fatalf("route scan found no tank routes among %v", protected)
	}

	a := newAPITestApp(t, config.Config{})
	newClient(t, a).setup(nil)
	anon := newClient(t, a)
	for _, pattern := range protected {
		method, target, _ := strings.Cut(pathWildcard.ReplaceAllString(pattern, "1"), " ")
		var body any
		if method != http.MethodGet && method != http.MethodDelete {
			body = map[string]any{}
		}
		rec := anon.do(method, target, body)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session: %d %s", pattern, rec.Code, rec.Body)
		}
	}
}

// TestFeatureRoutesLogUnexpectedFailuresToTheAppLogger breaks each feature's tables so its read fails unexpectedly, and expects the failure in the app's own log.
func TestFeatureRoutesLogUnexpectedFailuresToTheAppLogger(t *testing.T) {
	var logs bytes.Buffer
	database := openTestDB(t)
	store := settings.NewStore(database)
	hasher := password.NewHasher(password.Params{Time: 1, MemoryKiB: 64, Threads: 1})
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	// The backups folder sits below a regular file, so listing it fails with something other than a missing folder.
	dataFile := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(dataFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := newApp(config.Config{DataDir: dataFile}, logger, database, store, hasher, webui.Handler(logger))
	if err != nil {
		t.Fatal(err)
	}
	a := &testApp{App: app, db: database, store: store}
	c := newClient(t, a)
	c.setup(nil)
	tank := mustCreateTank(t, c)
	tankPath := "/api/v1/tanks/" + strconv.FormatInt(tank.ID, 10)
	rec := c.do(http.MethodPost, "/api/v1/maintenance/tasks", map[string]any{"name": "Filter", "schedule": map[string]any{"kind": "after_last", "interval_days": 7}})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	taskPath := "/api/v1/maintenance/tasks/" + strconv.FormatInt(testutil.Decode[struct {
		ID int64 `json:"id"`
	}](t, rec).ID, 10)

	testutil.Exec(t, a.db, `ALTER TABLE parameter_definitions RENAME TO parameter_definitions_gone`)
	testutil.Exec(t, a.db, `ALTER TABLE dashboard_cards RENAME TO dashboard_cards_gone`)
	testutil.Exec(t, a.db, `ALTER TABLE tanks RENAME TO tanks_gone`)
	testutil.Exec(t, a.db, `ALTER TABLE maintenance_presets RENAME TO maintenance_presets_gone`)
	testutil.Exec(t, a.db, `ALTER TABLE maintenance_tasks RENAME TO maintenance_tasks_gone`)
	testutil.Exec(t, a.db, `ALTER TABLE maintenance_completions RENAME TO maintenance_completions_gone`)
	testutil.Exec(t, a.db, `ALTER TABLE notification_channels RENAME TO notification_channels_gone`)
	testutil.Exec(t, a.db, `ALTER TABLE notifications RENAME TO notifications_gone`)
	for _, path := range []string{
		"/api/v1/parameters", tankPath + "/dashboard", "/api/v1/tanks",
		"/api/v1/maintenance/presets", "/api/v1/maintenance/tasks", taskPath, taskPath + "/completions",
		"/api/v1/notification-channels", "/api/v1/notifications", "/api/v1/notifications/unread-count", "/api/v1/backups",
	} {
		testutil.ExpectError(t, c.do(http.MethodGet, path, nil), http.StatusInternalServerError, "internal")
		if !loggedFailure(logs.String(), path) {
			t.Errorf("no failure logged for %s in:\n%s", path, logs.String())
		}
	}
}

// TestChannelAndBackupRoutesAreForAdminsOnly expects every channel and backup route to answer a signed-in member with 403.
func TestChannelAndBackupRoutesAreForAdminsOnly(t *testing.T) {
	_, protected := declaredAPIRoutes(t)
	var adminOnly []string
	for _, pattern := range protected {
		if strings.Contains(pattern, " /api/v1/notification-channels") || strings.Contains(pattern, " /api/v1/backups") {
			adminOnly = append(adminOnly, pattern)
		}
	}
	// Five channel routes and four backup routes.
	if len(adminOnly) != 9 {
		t.Fatalf("route scan found %d channel and backup routes, want 9: %v", len(adminOnly), adminOnly)
	}

	a, admin := adminAndApp(t)
	addUser(t, admin, "member1", "member")
	member := signedInAs(t, a, "member1")
	for _, pattern := range adminOnly {
		method, target, _ := strings.Cut(pathWildcard.ReplaceAllString(pattern, "1"), " ")
		var body any
		if method != http.MethodGet && method != http.MethodDelete {
			body = map[string]any{}
		}
		testutil.ExpectError(t, member.do(method, target, body), http.StatusForbidden, "forbidden")
	}

	// The inbox and the maintenance lists stay open to members.
	for _, target := range []string{"/api/v1/notifications", "/api/v1/notifications/unread-count", "/api/v1/maintenance/presets", "/api/v1/maintenance/tasks"} {
		testutil.ExpectStatus(t, member.do(http.MethodGet, target, nil), http.StatusOK)
	}
}

// TestMembersCanEditMaintenancePresets covers the one maintenance write that earlier checks only ran as an admin.
func TestMembersCanEditMaintenancePresets(t *testing.T) {
	a, admin := adminAndApp(t)
	addUser(t, admin, "member1", "member")
	member := signedInAs(t, a, "member1")

	type preset struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		ArchivedAt  *string `json:"archived_at"`
	}
	list := testutil.DecodeItems[preset](t, member.do(http.MethodGet, "/api/v1/maintenance/presets", nil))
	if len(list) == 0 {
		t.Fatal("no presets to edit")
	}
	path := "/api/v1/maintenance/presets/" + strconv.FormatInt(list[0].ID, 10)

	rec := member.do(http.MethodPatch, path, map[string]any{"name": "Renamed by a member", "description": "Edited"})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	var got preset
	testutil.DecodeInto(t, rec, &got)
	if got.ID != list[0].ID || got.Name != "Renamed by a member" || got.Description != "Edited" {
		t.Errorf("patched preset = %+v", got)
	}

	// Archiving hides the template from the default list, and a member can bring it back.
	testutil.ExpectStatus(t, member.do(http.MethodPatch, path, map[string]any{"archived": true}), http.StatusOK)
	for _, p := range testutil.DecodeItems[preset](t, member.do(http.MethodGet, "/api/v1/maintenance/presets", nil)) {
		if p.ID == got.ID {
			t.Error("the archived preset is still in the default list")
		}
	}
	testutil.ExpectStatus(t, member.do(http.MethodPatch, path, map[string]any{"archived": false}), http.StatusOK)

	// The change is visible to the admin, so it was stored and not just echoed.
	var seen *preset
	for _, p := range testutil.DecodeItems[preset](t, admin.do(http.MethodGet, "/api/v1/maintenance/presets", nil)) {
		if p.ID == got.ID {
			seen = &p
		}
	}
	if seen == nil || seen.Name != "Renamed by a member" || seen.ArchivedAt != nil {
		t.Errorf("preset as the admin sees it = %+v", seen)
	}

	testutil.ExpectError(t, member.do(http.MethodPatch, path, map[string]any{"name": ""}), http.StatusUnprocessableEntity, "validation", "name")
}

// loggedFailure reports whether log has an unexpected-failure line for path.
func loggedFailure(log, path string) bool {
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, `msg="request failed"`) && strings.Contains(line, "path="+path+" ") {
			return true
		}
	}
	return false
}
