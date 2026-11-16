package notify

import (
	"encoding/json"
	"testing"
)

func TestRegisterMakesAKindFindableAndRejectsDuplicates(t *testing.T) {
	const name = "registry_check"
	t.Cleanup(func() {
		registry.mu.Lock()
		delete(registry.kinds, name)
		registry.mu.Unlock()
	})
	if _, ok := lookup(name); ok {
		t.Fatalf("kind %q present before it was registered", name)
	}

	Register(Kind{Name: name, Secrets: []string{"url"}, Build: func(json.RawMessage, Env) (Notifier, error) { return nil, nil }})
	k, ok := lookup(name)
	if !ok || k.Name != name || len(k.Secrets) != 1 || k.Secrets[0] != "url" {
		t.Errorf("lookup = %+v, %v", k, ok)
	}

	defer func() {
		if recover() == nil {
			t.Error("registering the same kind twice did not panic")
		}
	}()
	Register(Kind{Name: name})
}

func TestRegisterRejectsAnEmptyName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("registering a kind without a name did not panic")
		}
	}()
	Register(Kind{})
}
