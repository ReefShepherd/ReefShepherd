// Package notify raises notifications, stores them for the in-app inbox and delivers them to the configured channels.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event is what a feature raises; the text is already rendered.
type Event struct {
	Type       string // maintenance_due, tests_due, out_of_range, backup_failed
	Severity   string // info, warning, critical
	Title      string // already rendered text
	Body       string
	Link       string // in-app path such as /tanks/1/maintenance
	TankID     *int64
	SourceKind string
	SourceID   int64
	Occurrence string
}

// Raiser stores notifications; features depend on this and not on the service.
type Raiser interface {
	// Raise stores the notification and queues deliveries; it reports false when this occurrence was already raised.
	Raise(ctx context.Context, e Event) (bool, error)
}

// Message is what a channel receives.
type Message struct {
	EventType, Severity, Title, Body string
	Link                             string // absolute URL from REEFSHEPHERD_PUBLIC_URL plus the path, empty when no public URL is set
	TankID                           int64  // 0 when the notification is not about one tank
	TankName                         string
	OccurredAt                       time.Time
	Instance                         string // instance name, for providers that show a sender or footer
}

// Notifier sends a message to one configured channel.
type Notifier interface {
	Send(ctx context.Context, msg Message) error
}

// Env is what a provider needs besides its own configuration.
type Env struct {
	HTTP      *http.Client // 10 s timeout
	UserAgent string       // ReefShepherd/<version>
}

// Kind describes one channel type.
type Kind struct {
	Name    string   // discord, slack, ntfy, webhook, email
	Secrets []string // top-level config fields masked on read, such as url, token or password
	// Build validates a config and returns a sender; validation problems are httpx.FieldErrors under "config.<field>".
	Build func(config json.RawMessage, env Env) (Notifier, error)
}

var registry = struct {
	mu    sync.RWMutex
	kinds map[string]Kind
}{kinds: map[string]Kind{}}

// Register adds a channel kind; each provider file calls it from init, and a duplicate name panics.
func Register(k Kind) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if k.Name == "" {
		panic("notify: channel kind without a name")
	}
	if _, dup := registry.kinds[k.Name]; dup {
		panic(fmt.Sprintf("notify: channel kind %q registered twice", k.Name))
	}
	registry.kinds[k.Name] = k
}

// lookup returns the registered kind with the given name.
func lookup(name string) (Kind, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	k, ok := registry.kinds[name]
	return k, ok
}
