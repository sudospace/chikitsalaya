// Package notify defines the outbound-SMS boundary used by online booking's
// OTP verification, kept tiny so a real provider can be dropped in later.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type SMSSender interface {
	Send(toMobile, message string) error
}

// ConsoleSender is the dev-mode fallback: logs the message instead of
// sending it, used when a company hasn't configured a real SMS integration.
type ConsoleSender struct{}

func (ConsoleSender) Send(toMobile, message string) error {
	log.Printf("[dev-mode SMS] to %s: %s", toMobile, message)
	return nil
}

// WebhookSender is the one real SMS mechanism shipped: point it at any
// HTTP endpoint accepting a plain JSON webhook — no vendor SDK needed.
type WebhookSender struct {
	URL            string
	AuthHeaderName string
	AuthHeaderVal  string
	Client         *http.Client
}

func (w WebhookSender) Send(toMobile, message string) error {
	if w.URL == "" {
		return fmt.Errorf("sms webhook: no URL configured")
	}
	body, err := json.Marshal(map[string]string{"to": toMobile, "message": message})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.AuthHeaderName != "" {
		req.Header.Set(w.AuthHeaderName, w.AuthHeaderVal)
	}

	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sms webhook returned status %d", resp.StatusCode)
	}
	return nil
}
