package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

const (
	// webhookVersion is the version of the JSON body a generic webhook receives.
	webhookVersion = 1
	// maxChannelURL is the longest URL a channel accepts.
	maxChannelURL = 2048
	// maxReplyRead is how much of a provider's answer is read.
	maxReplyRead = 4096
	// maxReplyDetail is how many characters of a provider's answer an error keeps.
	maxReplyDetail = 200
	// ellipsis marks text that was cut to fit a provider's limit.
	ellipsis = "…"

	// minHiddenHost is the shortest host name an error text hides.
	minHiddenHost = 4

	configSummary = "invalid channel configuration"
)

func init() {
	Register(Kind{Name: "webhook", Secrets: []string{"url"}, Build: webhookBuild})
}

type webhookConfig struct {
	URL string `json:"url"`
}

type webhookNotifier struct {
	env Env
	url string
}

func webhookBuild(config json.RawMessage, env Env) (Notifier, error) {
	var cfg webhookConfig
	if err := decodeChannelConfig(config, &cfg); err != nil {
		return nil, err
	}
	f := httpx.FieldErrors{Message: configSummary}
	target := cleanChannelURL(cfg.URL, "config.url", &f)
	if err := f.Err(); err != nil {
		return nil, err
	}
	return &webhookNotifier{env: env, url: target}, nil
}

// webhookTank is the tank a generic webhook payload is about.
type webhookTank struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// webhookPayload is the versioned JSON body a generic webhook receives.
type webhookPayload struct {
	Version    int          `json:"version"`
	Event      string       `json:"event"`
	Severity   string       `json:"severity"`
	Title      string       `json:"title"`
	Body       string       `json:"body"`
	Link       string       `json:"link"`
	Tank       *webhookTank `json:"tank"`
	OccurredAt *string      `json:"occurred_at"`
}

func (n *webhookNotifier) Send(ctx context.Context, msg Message) error {
	payload := webhookPayload{
		Version: webhookVersion, Event: msg.EventType, Severity: msg.Severity,
		Title: msg.Title, Body: msg.Body, Link: msg.Link,
	}
	if msg.TankID != 0 {
		payload.Tank = &webhookTank{ID: msg.TankID, Name: msg.TankName}
	}
	if !msg.OccurredAt.IsZero() {
		at := msg.OccurredAt.UTC().Format(time.RFC3339)
		payload.OccurredAt = &at
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: encode payload: %w", err)
	}
	reply, err := postJSON(ctx, n.env, n.url, nil, body)
	if err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	if reply.ok() {
		return nil
	}
	return replyFailure("webhook", reply, false, n.url)
}

// decodeChannelConfig reads a channel config into dst, refusing unknown fields, wrong types and trailing data.
func decodeChannelConfig(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			field, _, _ := strings.Cut(typeErr.Field, ".")
			return httpx.Reject(configSummary, "config."+field, httpx.CodeInvalid)
		}
		return httpx.Reject(configSummary, "config", httpx.CodeInvalid)
	}
	if dec.More() {
		return httpx.Reject(configSummary, "config", httpx.CodeInvalid)
	}
	return nil
}

// cleanChannelURL checks an http or https URL and returns it trimmed, recording why it cannot be used under field.
func cleanChannelURL(raw, field string, f *httpx.FieldErrors) string {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		f.Add(field, httpx.CodeRequired)
	case len(raw) > maxChannelURL:
		f.Add(field, httpx.CodeTooLong)
	default:
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			f.Add(field, httpx.CodeInvalid)
		}
	}
	return raw
}

// httpReply is what a provider answered.
type httpReply struct {
	status int
	header http.Header
	body   []byte
}

func (r httpReply) ok() bool { return r.status >= 200 && r.status < 300 }

// postJSON posts body to target and returns the answer without following redirects, since a redirected POST can silently turn into a GET that delivers nothing.
func postJSON(ctx context.Context, env Env, target string, header http.Header, body []byte) (httpReply, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	if env.HTTP != nil {
		copied := *env.HTTP
		client = &copied
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return httpReply{}, transportFailure(err, target)
	}
	for name, values := range header {
		req.Header[name] = values
	}
	req.Header.Set("Content-Type", "application/json")
	if env.UserAgent != "" {
		req.Header.Set("User-Agent", env.UserAgent)
	}
	resp, err := client.Do(req)
	if err != nil {
		return httpReply{}, transportFailure(err, target)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReplyRead))
	if err != nil {
		return httpReply{}, transportFailure(err, target)
	}
	return httpReply{status: resp.StatusCode, header: resp.Header, body: data}, nil
}

// scrubbedError is a transport error whose text names neither the request URL nor its host, with the cause kept in the chain.
type scrubbedError struct {
	text  string
	cause error
}

func (e *scrubbedError) Error() string { return e.text }
func (e *scrubbedError) Unwrap() error { return e.cause }

// transportFailure reports a request that got no answer as its operation and cause only, since the URL (and sometimes the host) is a secret.
func transportFailure(err error, target string) error {
	op := "request"
	cause := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		op, cause = strings.ToLower(urlErr.Op), urlErr.Err
	}
	text := op + ": " + cause.Error()
	// A name of a few characters is left alone, since replacing it would garble the rest of the text.
	if u, perr := url.Parse(target); perr == nil && len(u.Hostname()) >= minHiddenHost {
		text = strings.ReplaceAll(text, u.Hostname(), "host")
	}
	return &scrubbedError{text: text, cause: cause}
}

// throttledError is a send that the provider asked to be repeated later.
type throttledError struct {
	text  string
	after time.Duration
}

func (e *throttledError) Error() string             { return e.text }
func (e *throttledError) RetryAfter() time.Duration { return e.after }

// goneError is a send to a destination that no longer exists.
type goneError struct{ text string }

func (e *goneError) Error() string { return e.text }
func (e *goneError) Gone() bool    { return true }

// replyFailure turns a non-2xx answer into an error holding the status and the provider's short error text, with the secrets the answer may echo masked before it is cut.
func replyFailure(provider string, r httpReply, gone bool, secrets ...string) error {
	text := provider + ": HTTP " + strconv.Itoa(r.status)
	if detail := replyDetail(r.body, secrets); detail != "" {
		text += ": " + detail
	}
	switch {
	case gone:
		return &goneError{text: text}
	case r.status == http.StatusTooManyRequests || (r.status == http.StatusServiceUnavailable && r.header.Get("Retry-After") != ""):
		return &throttledError{text: text, after: replyRetryAfter(r)}
	}
	return errors.New(text)
}

// replyDetail is the short error text of an answer: its message or error field when it is JSON, otherwise the text itself, with secrets masked, on one line and cut short.
func replyDetail(body []byte, secrets []string) string {
	text := string(body)
	var fields map[string]any
	if json.Unmarshal(body, &fields) == nil {
		for _, key := range []string{"message", "error", "detail"} {
			if s, ok := fields[key].(string); ok && s != "" {
				text = s
				break
			}
		}
	}
	text = strings.Map(func(r rune) rune {
		if r < ' ' && r != '\t' && r != '\n' && r != '\r' {
			return -1
		}
		return r
	}, strings.ToValidUTF8(text, "�"))
	// Masking comes first because a cut through the middle of a secret would leave a piece nothing can recognise.
	text = maskSecrets(text, secrets)
	return clipRunes(strings.Join(strings.Fields(text), " "), maxReplyDetail)
}

// maskSecrets replaces every secret in text, and for a URL its path, segments and query, with the mask.
func maskSecrets(text string, secrets []string) string {
	terms := redactionTerms(secrets)
	if len(terms) == 0 {
		return text
	}
	pairs := make([]string, 0, 2*len(terms))
	for _, t := range terms {
		pairs = append(pairs, t, maskedSecret)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// replyRetryAfter is how long a throttled provider asked to wait: the retry_after number of a JSON answer, else the Retry-After header; zero when it said nothing usable.
func replyRetryAfter(r httpReply) time.Duration {
	var fields struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if json.Unmarshal(r.body, &fields) == nil && fields.RetryAfter > 0 {
		return secondsToDuration(fields.RetryAfter)
	}
	header := strings.TrimSpace(r.header.Get("Retry-After"))
	if seconds, err := strconv.ParseFloat(header, 64); err == nil && seconds > 0 {
		return secondsToDuration(seconds)
	}
	if at, err := http.ParseTime(header); err == nil {
		return max(time.Until(at), 0)
	}
	return 0
}

// secondsToDuration converts a provider's delay, capped so a huge number cannot overflow.
func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(min(seconds, maxRetryAfter.Seconds()) * float64(time.Second))
}

// clipRunes keeps s within n characters, ending it with an ellipsis when it had to be cut.
func clipRunes(s string, n int) string {
	s = strings.ToValidUTF8(s, "�")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return truncate(s, n-1) + ellipsis
}

// clipBytes keeps s within n bytes without splitting a character, ending it with an ellipsis when it had to be cut.
func clipBytes(s string, n int) string {
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= n {
		return s
	}
	cut := n - len(ellipsis)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + ellipsis
}
