package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// Discord's limits for the parts of a message this provider fills in.
const (
	discordMaxContent     = 2000
	discordMaxTitle       = 256
	discordMaxDescription = 4096
	discordMaxFooter      = 256
)

// Embed colours by severity, as the integer value of 0xRRGGBB.
const (
	discordColorInfo     = 0x3498db
	discordColorWarning  = 0xf39c12
	discordColorCritical = 0xe74c3c
)

func init() {
	Register(Kind{Name: "discord", Secrets: []string{"url"}, Build: discordBuild})
}

type discordConfig struct {
	URL string `json:"url"`
}

type discordNotifier struct {
	env Env
	url string
}

func discordBuild(config json.RawMessage, env Env) (Notifier, error) {
	var cfg discordConfig
	if err := decodeChannelConfig(config, &cfg); err != nil {
		return nil, err
	}
	f := httpx.FieldErrors{Message: configSummary}
	target := cleanChannelURL(cfg.URL, "config.url", &f)
	if err := f.Err(); err != nil {
		return nil, err
	}
	return &discordNotifier{env: env, url: target}, nil
}

type discordPayload struct {
	Content         string          `json:"content"`
	Embeds          []discordEmbed  `json:"embeds"`
	AllowedMentions discordMentions `json:"allowed_mentions"`
}

// discordMentions says which mentions in the text may notify anyone; an empty parse list means none.
type discordMentions struct {
	Parse []string `json:"parse"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Color       int            `json:"color"`
	Timestamp   string         `json:"timestamp,omitempty"`
	Footer      *discordFooter `json:"footer,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

func discordColor(severity string) int {
	switch severity {
	case "critical":
		return discordColorCritical
	case "warning":
		return discordColorWarning
	}
	return discordColorInfo
}

func (n *discordNotifier) Send(ctx context.Context, msg Message) error {
	embed := discordEmbed{
		Title:       clipRunes(msg.Title, discordMaxTitle),
		Description: clipRunes(msg.Body, discordMaxDescription),
		URL:         msg.Link,
		Color:       discordColor(msg.Severity),
	}
	if !msg.OccurredAt.IsZero() {
		embed.Timestamp = msg.OccurredAt.UTC().Format(time.RFC3339)
	}
	if msg.Instance != "" {
		embed.Footer = &discordFooter{Text: clipRunes(msg.Instance, discordMaxFooter)}
	}
	body, err := json.Marshal(discordPayload{
		Content:         clipRunes(msg.Title, discordMaxContent),
		Embeds:          []discordEmbed{embed},
		AllowedMentions: discordMentions{Parse: []string{}},
	})
	if err != nil {
		return fmt.Errorf("discord: encode payload: %w", err)
	}
	reply, err := postJSON(ctx, n.env, n.url, nil, body)
	if err != nil {
		return fmt.Errorf("discord: %w", err)
	}
	if reply.ok() {
		return nil
	}
	return replyFailure("discord", reply, reply.status == http.StatusNotFound, n.url)
}
