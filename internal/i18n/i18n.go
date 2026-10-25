// Package i18n looks up server-side message text, such as notification and email bodies, by locale.
package i18n

import "fmt"

// DefaultLocale is used when a locale or a key is missing.
const DefaultLocale = "en"

// Catalog maps locale, then message key, to a fmt format string.
type Catalog struct {
	messages map[string]map[string]string
}

// New builds a catalog from messages[locale][key].
func New(messages map[string]map[string]string) *Catalog {
	return &Catalog{messages: messages}
}

// T returns the message for key in locale formatted with args, falling back to the default locale and then to the key itself.
func (c *Catalog) T(locale, key string, args ...any) string {
	format, ok := c.messages[locale][key]
	if !ok {
		format, ok = c.messages[DefaultLocale][key]
	}
	if !ok {
		return key
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
