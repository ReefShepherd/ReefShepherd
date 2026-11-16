package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

//go:embed messages
var embedded embed.FS

var defaultCatalog = mustLoad(embedded)

// Default is the catalog built from the embedded message files.
func Default() *Catalog {
	return defaultCatalog
}

func mustLoad(fsys fs.FS) *Catalog {
	c, err := load(fsys)
	if err != nil {
		panic("i18n: " + err.Error())
	}
	return c
}

// load reads flat key-to-format JSON files from messages/<locale>/ in fsys and rejects unparsable files, duplicate keys, a missing default locale and locales missing a default key.
func load(fsys fs.FS) (*Catalog, error) {
	locales, err := fs.ReadDir(fsys, "messages")
	if err != nil {
		return nil, fmt.Errorf("read messages: %w", err)
	}
	messages := map[string]map[string]string{}
	for _, entry := range locales {
		if !entry.IsDir() {
			continue
		}
		keys, err := loadLocale(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		messages[entry.Name()] = keys
	}

	base, ok := messages[DefaultLocale]
	if !ok {
		return nil, fmt.Errorf("no messages for the default locale %q", DefaultLocale)
	}
	for locale, keys := range messages {
		var missing []string
		for key := range base {
			if _, ok := keys[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			slices.Sort(missing)
			return nil, fmt.Errorf("locale %q lacks keys of %q: %s", locale, DefaultLocale, strings.Join(missing, ", "))
		}
	}
	return New(messages), nil
}

func loadLocale(fsys fs.FS, locale string) (map[string]string, error) {
	dir := path.Join("messages", locale)
	files, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	keys := map[string]string{}
	owner := map[string]string{}
	for _, f := range files {
		if f.IsDir() || path.Ext(f.Name()) != ".json" {
			continue
		}
		name := path.Join(dir, f.Name())
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var entries map[string]string
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		for key, format := range entries {
			if first, dup := owner[key]; dup {
				return nil, fmt.Errorf("key %q in %s is already defined in %s", key, name, first)
			}
			owner[key] = name
			keys[key] = format
		}
	}
	return keys, nil
}
