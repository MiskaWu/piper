package digest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// discordWebhookPayload is the Discord webhook API payload.
type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Color       int                 `json:"color"`
	Fields      []discordEmbedField `json:"fields,omitempty"`
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// FormatDuration formats seconds into "Xh Ym" human-readable string.
func FormatDuration(sec int) string {
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// FormatDiscord converts a Digest into a Discord webhook payload.
func FormatDiscord(d *Digest) discordWebhookPayload {
	var desc strings.Builder
	fmt.Fprintf(&desc, "**Work Time:** %s\n", FormatDuration(d.TotalWorkSec))

	embed := discordEmbed{
		Title:       fmt.Sprintf("Daily Digest — %s", d.Date),
		Description: desc.String(),
		Color:       0x7C3AED, // purple
	}

	for _, proj := range d.Projects {
		name := proj.ProjectName
		if name == "" {
			name = "(no project)"
		}

		var val strings.Builder
		if proj.Summary != "" {
			val.WriteString(proj.Summary)
		} else {
			// Fallback: list session descriptions when no AI summary
			for _, sess := range proj.Sessions {
				d := sess.Description
				if d == "" {
					d = sess.Name
				}
				fmt.Fprintf(&val, "- %s\n", d)
			}
		}

		embed.Fields = append(embed.Fields, discordEmbedField{
			Name:  fmt.Sprintf("%s (%s)", name, FormatDuration(proj.TotalWorkSec)),
			Value: val.String(),
		})
	}

	if len(d.Projects) == 0 {
		embed.Description += "\nNo activity recorded."
	}

	return discordWebhookPayload{Embeds: []discordEmbed{embed}}
}

// HTTPClient allows injecting a custom HTTP client for testing.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// SendWebhook sends the digest to a Discord webhook URL.
func SendWebhook(client HTTPClient, webhookURL string, d *Digest) error {
	payload := FormatDiscord(d)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
