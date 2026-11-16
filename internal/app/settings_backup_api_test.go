package app

import (
	"net/http"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/config"
	"github.com/reefshepherd/reefshepherd/internal/testutil"
)

type backupSettings struct {
	BackupEnabled bool   `json:"backup_enabled"`
	BackupTime    string `json:"backup_time"`
	BackupKeep    int    `json:"backup_keep"`
}

func getBackupSettings(t *testing.T, c *apiClient) backupSettings {
	t.Helper()
	rec := c.do(http.MethodGet, "/api/v1/settings", nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	return testutil.Decode[backupSettings](t, rec)
}

func TestBackupSettingsInGetAndPatch(t *testing.T) {
	_, c := adminAndApp(t)
	if got := getBackupSettings(t, c); got != (backupSettings{BackupEnabled: true, BackupTime: "03:30", BackupKeep: 14}) {
		t.Errorf("defaults = %+v", got)
	}

	rec := c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"backup_enabled": false, "backup_time": "01:45", "backup_keep": 30})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	want := backupSettings{BackupEnabled: false, BackupTime: "01:45", BackupKeep: 30}
	if got := testutil.Decode[backupSettings](t, rec); got != want {
		t.Errorf("patched = %+v, want %+v", got, want)
	}
	if got := getBackupSettings(t, c); got != want {
		t.Errorf("persisted = %+v, want %+v", got, want)
	}

	// A patch that names other fields leaves the backup settings alone.
	testutil.ExpectStatus(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"instance_name": "Reef"}), http.StatusOK)
	if got := getBackupSettings(t, c); got != want {
		t.Errorf("after an unrelated patch = %+v, want %+v", got, want)
	}
}

func TestBackupSettingsReportFieldCodesAndApplyNothingOnError(t *testing.T) {
	_, c := adminAndApp(t)
	b := testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{
		"backup_enabled": false, "backup_time": "25:00", "backup_keep": 366,
	}), http.StatusUnprocessableEntity, "validation")
	expectCodes(t, b, map[string]string{"backup_time": "invalid", "backup_keep": "out_of_range"})
	expectCodes(t, testutil.ExpectError(t, c.do(http.MethodPatch, "/api/v1/settings", map[string]any{"backup_keep": 0}), http.StatusUnprocessableEntity, "validation"), map[string]string{"backup_keep": "out_of_range"})
	if got := getBackupSettings(t, c); !got.BackupEnabled {
		t.Errorf("a rejected patch was partly applied: %+v", got)
	}
}

func TestBackupRoutesAreAdminOnlyThroughTheWholeApp(t *testing.T) {
	a := newAPITestApp(t, config.Config{DataDir: t.TempDir()})
	admin := newClient(t, a)
	admin.setup(nil)
	addUser(t, admin, "mia", "member")
	member := signedInAs(t, a, "mia")

	testutil.ExpectError(t, member.do(http.MethodPost, "/api/v1/backups", nil), http.StatusForbidden, "forbidden")
	testutil.ExpectError(t, member.do(http.MethodGet, "/api/v1/backups", nil), http.StatusForbidden, "forbidden")

	rec := admin.do(http.MethodPost, "/api/v1/backups", nil)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	items := testutil.DecodeItems[struct {
		Name string `json:"name"`
	}](t, admin.do(http.MethodGet, "/api/v1/backups", nil))
	if len(items) != 1 {
		t.Errorf("backups = %v, want the one just made", items)
	}
}
