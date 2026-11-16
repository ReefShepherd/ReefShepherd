package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

const (
	// slackMaxText is the length Slack recommends for message text, in characters.
	slackMaxText = 4000
	// slackMaxTitle keeps a long title from using up the room for the body.
	slackMaxTitle = 500
	// slackLinkText is what the link to the app reads as.
	slackLinkText = "Open"
)

// slackGone are the answers that mean the webhook was removed or its channel is gone.
var slackGone = []string{"no_service", "no_service_id", "no_active_hooks", "channel_not_found"}

func init() {
	Register(Kind{Name: "slack", Secrets: []string{"url"}, Build: slackBuild})
}

type slackConfig struct {
	URL string `json:"url"`
}

type slackNotifier struct {
	env Env
	url string
}

func slackBuild(config json.RawMessage, env Env) (Notifier, error) {
	var cfg slackConfig
	if err := decodeChannelConfig(config, &cfg); err != nil {
		return nil, err
	}
	f := httpx.FieldErrors{Message: configSummary}
	target := cleanChannelURL(cfg.URL, "config.url", &f)
	if err := f.Err(); err != nil {
		return nil, err
	}
	return &slackNotifier{env: env, url: target}, nil
}

// slackPiece is a character as Slack wants it: the three characters it treats as markup become entities.
func slackPiece(r rune) string {
	switch r {
	case '&':
		return "&amp;"
	case '<':
		return "&lt;"
	case '>':
		return "&gt;"
	}
	return string(r)
}

// slackEscape escapes s for Slack and keeps the result within limit characters, ending it with an ellipsis rather than cutting an entity in two.
func slackEscape(s string, limit int) string {
	s = strings.ToValidUTF8(s, "�")
	var b strings.Builder
	used := 0
	for _, r := range s {
		b.WriteString(slackPiece(r))
		used += utf8.RuneCountInString(slackPiece(r))
	}
	if used <= limit {
		return b.String()
	}
	b.Reset()
	used = 0
	for _, r := range s {
		piece := slackPiece(r)
		width := utf8.RuneCountInString(piece)
		if used+width > limit-1 {
			break
		}
		b.WriteString(piece)
		used += width
	}
	return b.String() + ellipsis
}

// slackLink writes a link in Slack's <url|text> form, escaping the characters that would end it early.
func slackLink(link string) string {
	link = strings.NewReplacer("|", "%7C", "<", "%3C", ">", "%3E", " ", "%20", "\r", "", "\n", "").Replace(link)
	return "<" + link + "|" + slackLinkText + ">"
}

// slackText builds the message: the title in bold, the body and the link when there is one.
func slackText(msg Message) string {
	var tail string
	if msg.Link != "" {
		tail = "\n" + slackLink(msg.Link)
	}
	head := "*" + slackEscape(msg.Title, slackMaxTitle) + "*"
	room := slackMaxText - utf8.RuneCountInString(head) - utf8.RuneCountInString(tail) - len("\n")
	if msg.Body == "" || room < 1 {
		return head + tail
	}
	return head + "\n" + slackEscape(msg.Body, room) + tail
}

func (n *slackNotifier) Send(ctx context.Context, msg Message) error {
	body, err := json.Marshal(map[string]string{"text": slackText(msg)})
	if err != nil {
		return fmt.Errorf("slack: encode payload: %w", err)
	}
	reply, err := postJSON(ctx, n.env, n.url, nil, body)
	if err != nil {
		return fmt.Errorf("slack: %w", err)
	}
	if reply.ok() {
		return nil
	}
	gone := reply.status == http.StatusNotFound || slices.Contains(slackGone, strings.TrimSpace(string(reply.body)))
	return replyFailure("slack", reply, gone, n.url)
}
