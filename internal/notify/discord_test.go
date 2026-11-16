package notify

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// discordServer starts a server standing in for a Discord webhook and builds the channel that posts to it.
func discordServer(t *testing.T, replies ...webhookReply) (Notifier, func() []webhookCall) {
	t.Helper()
	srv, calls := webhookServer(t, replies...)
	return webhookBuilt(t, "discord", `{"url":"`+srv.URL+`/api/webhooks/123/tok-abc?thread_id=9"}`, webhookEnv(srv)), calls
}

func TestDiscordRequest(t *testing.T) {
	n, calls := discordServer(t, webhookReply{Status: 204})
	if err := webhookSend(t, n, webhookMessage()); err != nil {
		t.Fatal(err)
	}
	c := calls()[0]
	if c.Method != http.MethodPost || c.Path != "/api/webhooks/123/tok-abc" || c.RawQuery != "thread_id=9" {
		t.Errorf("request = %s %s?%s", c.Method, c.Path, c.RawQuery)
	}
	if c.Header.Get("Content-Type") != "application/json" || c.Header.Get("User-Agent") != "ReefShepherd/test" {
		t.Errorf("headers = %v", c.Header)
	}
	webhookSameJSON(t, c.Body, `{
		"content": "Water change due",
		"embeds": [{
			"title": "Water change due",
			"description": "Tank Reef needs a 20% water change.",
			"url": "https://reef.example.com/tanks/1/maintenance",
			"color": 15965202,
			"timestamp": "2026-11-15T09:00:00Z",
			"footer": {"text": "My Reef"}
		}],
		"allowed_mentions": {"parse": []}
	}`)
}

func TestDiscordSuppressesMentions(t *testing.T) {
	n, calls := discordServer(t, webhookReply{Status: 204})
	msg := webhookMessage()
	msg.Title, msg.Body = "@everyone <@123456789> <@&987654321>", "@here look"
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls()[0].Body), `"allowed_mentions":{"parse":[]}`) {
		t.Errorf("body does not switch every mention off: %s", calls()[0].Body)
	}
}

func TestDiscordColors(t *testing.T) {
	want := map[string]int{"info": 0x3498db, "warning": 0xf39c12, "critical": 0xe74c3c, "": 0x3498db, "weird": 0x3498db}
	for severity, color := range want {
		n, calls := discordServer(t, webhookReply{Status: 204})
		msg := webhookMessage()
		msg.Severity = severity
		if err := webhookSend(t, n, msg); err != nil {
			t.Fatal(err)
		}
		var got struct{ Embeds []struct{ Color int } }
		if err := json.Unmarshal(calls()[0].Body, &got); err != nil || got.Embeds[0].Color != color {
			t.Errorf("severity %q: color = %v, want %d (%v)", severity, got, color, err)
		}
	}
}

func TestDiscordOmitsWhatIsMissing(t *testing.T) {
	n, calls := discordServer(t, webhookReply{Status: 204})
	if err := webhookSend(t, n, Message{EventType: "backup_failed", Severity: "info", Title: "Hello"}); err != nil {
		t.Fatal(err)
	}
	webhookSameJSON(t, calls()[0].Body, `{
		"content": "Hello",
		"embeds": [{"title": "Hello", "color": 3447003}],
		"allowed_mentions": {"parse": []}
	}`)
}

func TestDiscordTruncatesByCharacter(t *testing.T) {
	n, calls := discordServer(t, webhookReply{Status: 204})
	msg := webhookMessage()
	msg.Title = strings.Repeat("🐠", 2500)
	msg.Body = strings.Repeat("é", 5000)
	msg.Instance = strings.Repeat("ü", 400)
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Content string
		Embeds  []struct {
			Title, Description string
			Footer             struct{ Text string }
		}
	}
	if err := json.Unmarshal(calls()[0].Body, &got); err != nil {
		t.Fatal(err)
	}
	e := got.Embeds[0]
	for name, c := range map[string]struct {
		text string
		max  int
	}{"content": {got.Content, 2000}, "title": {e.Title, 256}, "description": {e.Description, 4096}, "footer": {e.Footer.Text, 256}} {
		if n := utf8.RuneCountInString(c.text); n != c.max || !strings.HasSuffix(c.text, ellipsis) || !utf8.ValidString(c.text) {
			t.Errorf("%s has %d characters (valid=%v), want %d ending in an ellipsis", name, n, utf8.ValidString(c.text), c.max)
		}
	}
	// A short message is sent whole.
	n, calls = discordServer(t, webhookReply{Status: 204})
	msg = webhookMessage()
	msg.Body = strings.Repeat("é", 4096)
	if err := webhookSend(t, n, msg); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(calls()[0].Body, &got)
	if got.Embeds[0].Description != msg.Body {
		t.Error("a body at the limit was cut")
	}
}

func TestDiscordFailures(t *testing.T) {
	t.Run("rate limited", func(t *testing.T) {
		n, _ := discordServer(t, webhookReply{Status: 429, Header: map[string]string{"Retry-After": "3"}, Body: `{"message":"You are being rate limited.","retry_after":2.5,"global":false}`})
		err := webhookSend(t, n, webhookMessage())
		var throttled RetryAfterError
		if !errors.As(err, &throttled) || throttled.RetryAfter() != 2500*time.Millisecond {
			t.Fatalf("err = %v", err)
		}
		if !strings.HasPrefix(err.Error(), "discord: HTTP 429: You are being rate limited.") {
			t.Errorf("text = %v", err)
		}
		var gone GoneError
		if errors.As(err, &gone) {
			t.Error("a rate limit is not gone")
		}
	})
	t.Run("webhook deleted", func(t *testing.T) {
		n, _ := discordServer(t, webhookReply{Status: 404, Body: `{"message":"Unknown Webhook","code":10015}`})
		err := webhookSend(t, n, webhookMessage())
		var gone GoneError
		if !errors.As(err, &gone) || !gone.Gone() {
			t.Fatalf("err = %v, want a GoneError", err)
		}
		if err.Error() != "discord: HTTP 404: Unknown Webhook" {
			t.Errorf("text = %v", err)
		}
	})
	t.Run("bad request", func(t *testing.T) {
		n, _ := discordServer(t, webhookReply{Status: 400, Body: `{"message":"Invalid Form Body","code":50035,"errors":{"embeds":{}}}`})
		err := webhookSend(t, n, webhookMessage())
		var gone GoneError
		var throttled RetryAfterError
		if err == nil || err.Error() != "discord: HTTP 400: Invalid Form Body" || errors.As(err, &gone) || errors.As(err, &throttled) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestDiscordBuildValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		config string
		want   map[string]string
	}{
		"missing url": {`{}`, map[string]string{"config.url": "required"}},
		"bad url":     {`{"url":"nope"}`, map[string]string{"config.url": "invalid"}},
		"extra field": {`{"url":"https://discord.com/api/webhooks/1/x","name":"a"}`, map[string]string{"config": "invalid"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := webhookRefused(t, "discord", tc.config); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("fields = %v, want %v", got, tc.want)
			}
		})
	}
	webhookBuilt(t, "discord", `{"url":"http://localhost:9000/api/webhooks/1/x"}`, Env{})
}
