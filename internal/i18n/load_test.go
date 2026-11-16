package i18n

import (
	"strings"
	"testing"
	"testing/fstest"
)

func files(entries map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range entries {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestLoadMergesFilesOfALocale(t *testing.T) {
	c, err := load(files(map[string]string{
		"messages/en/common.json":      `{"app_name": "Reef"}`,
		"messages/en/maintenance.json": `{"maintenance_due_title": "%[1]s is due on %[2]s"}`,
		"messages/en/notes.txt":        `ignored, not json`,
		"messages/de/common.json":      `{"app_name": "Riff"}`,
		"messages/de/maintenance.json": `{"maintenance_due_title": "%[1]s ist am %[2]s fällig"}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.T("en", "maintenance_due_title", "Water change", "Monday"); got != "Water change is due on Monday" {
		t.Errorf("en = %q", got)
	}
	if got := c.T("de", "maintenance_due_title", "Wasserwechsel", "Montag"); got != "Wasserwechsel ist am Montag fällig" {
		t.Errorf("de = %q", got)
	}
	if got := c.T("de", "app_name"); got != "Riff" {
		t.Errorf("de app_name = %q", got)
	}
}

func TestLoadRejectsBadMessageFiles(t *testing.T) {
	cases := []struct {
		name    string
		entries map[string]string
		want    string
	}{
		{
			name: "duplicate key across files",
			entries: map[string]string{
				"messages/en/a.json": `{"shared_key": "one"}`,
				"messages/en/b.json": `{"shared_key": "two"}`,
			},
			want: `key "shared_key"`,
		},
		{
			name:    "file that does not parse",
			entries: map[string]string{"messages/en/a.json": `{"app_name": `},
			want:    "parse messages/en/a.json",
		},
		{
			name:    "value that is not a string",
			entries: map[string]string{"messages/en/a.json": `{"count": 3}`},
			want:    "parse messages/en/a.json",
		},
		{
			name: "locale missing a key that en has",
			entries: map[string]string{
				"messages/en/a.json": `{"one": "1", "two": "2"}`,
				"messages/de/a.json": `{"one": "eins"}`,
			},
			want: `locale "de" lacks keys of "en": two`,
		},
		{
			name:    "no default locale",
			entries: map[string]string{"messages/de/a.json": `{"one": "eins"}`},
			want:    `default locale "en"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(files(tc.entries))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("load error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadAllowsExtraKeysInAnotherLocale(t *testing.T) {
	_, err := load(files(map[string]string{
		"messages/en/a.json": `{"one": "1"}`,
		"messages/de/a.json": `{"one": "eins", "extra": "mehr"}`,
	}))
	if err != nil {
		t.Errorf("load = %v, want no error", err)
	}
}

func TestDefaultServesTheEmbeddedMessages(t *testing.T) {
	if got := Default().T(DefaultLocale, "app_name"); got != "ReefShepherd" {
		t.Errorf("app_name = %q, want ReefShepherd", got)
	}
	first, second := Default(), Default()
	if first != second {
		t.Error("Default builds a new catalog on every call")
	}
}
