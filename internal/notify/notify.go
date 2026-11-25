package notify

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/azamasad/pingpong/internal/config"
	"github.com/azamasad/pingpong/internal/logx"
)

const (
	EventBatchDone      = "batch_done"
	EventScheduleFailed = "schedule_failed"
	EventJobFailed      = "job_failed"
)

// Client posts short plain-text messages to one optional webhook.
type Client struct {
	cfg      config.NotifyConfig
	client   *http.Client
	testPost func(event, body string) error
}

func New(cfg config.NotifyConfig) *Client {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout.Duration,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errRedirect
			},
		},
	}
}

var errRedirect = fmt.Errorf("redirect not allowed")

// Enabled reports whether a webhook URL is configured.
func (c *Client) Enabled() bool {
	return strings.TrimSpace(c.cfg.WebhookURL) != ""
}

// Wants reports whether event is listed and sending is on.
func (c *Client) Wants(event string) bool {
	if !c.Enabled() {
		return false
	}
	for _, e := range c.cfg.Events {
		if e == event {
			return true
		}
	}
	return false
}

// Send posts asynchronously when enabled. Never affects caller outcome.
func (c *Client) Send(event, body string) {
	if !c.Wants(event) {
		return
	}
	go c.deliver(event, body)
}

func (c *Client) deliver(event, body string) {
	if err := c.post(event, body); err != nil {
		logx.Event("warn", "notify_failed", map[string]any{"error_code": errorCode(err)})
	}
}

// PostSync is for tests: one attempt, same rules as production.
func (c *Client) PostSync(event, body string) error {
	if !c.Wants(event) {
		return nil
	}
	return c.post(event, body)
}

// SetTestPost overrides the HTTP sender (tests only).
func (c *Client) SetTestPost(fn func(event, body string) error) {
	c.testPost = fn
}

func (c *Client) post(event, body string) error {
	if c.testPost != nil {
		return c.testPost(event, body)
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimSpace(c.cfg.WebhookURL), bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", "Pingpong")
	req.Header.Set("X-Pingpong-Event", event)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently ||
		resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == http.StatusPermanentRedirect {
		return errRedirect
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	if err == errRedirect {
		return "NOTIFY_REDIRECT"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") {
		return "NOTIFY_TIMEOUT"
	}
	if strings.Contains(msg, "http 5") {
		return "NOTIFY_UPSTREAM"
	}
	if strings.Contains(msg, "http ") {
		return "NOTIFY_HTTP"
	}
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "dial") {
		return "NOTIFY_NETWORK"
	}
	return "NOTIFY_FAILED"
}

// ValidWebhookURL checks scheme and shape for doctor and config validation.
func ValidWebhookURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return true
	default:
		return false
	}
}
