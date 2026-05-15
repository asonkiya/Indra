// Package relay provides a client for the Indra push notification relay.
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client talks to the push notification relay server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a relay client pointing at the given base URL (e.g. "https://relay.indra.chat").
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Register tells the relay to associate a push token with a peer ID.
func (c *Client) Register(ctx context.Context, peerID, token, platform string) error {
	body, _ := json.Marshal(map[string]string{
		"peer_id":  peerID,
		"token":    token,
		"platform": platform,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("relay register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay register: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Notify asks the relay to send a silent push to the recipient.
func (c *Client) Notify(ctx context.Context, fromPeerID, toPeerID string) error {
	body, _ := json.Marshal(map[string]string{
		"from": fromPeerID,
		"to":   toPeerID,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/notify", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("relay notify: %w", err)
	}
	defer resp.Body.Close()

	// 404 = peer not registered, not an error worth propagating.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("relay notify: HTTP %d", resp.StatusCode)
}
