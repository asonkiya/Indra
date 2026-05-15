package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// APNsPusher sends silent push notifications via Apple's HTTP/2 APNs API.
type APNsPusher struct {
	key    *ecdsa.PrivateKey
	keyID  string
	teamID string
	topic  string // bundle ID
	prod   bool   // true = api.push.apple.com, false = api.sandbox.push.apple.com

	mu        sync.Mutex
	token     string
	tokenTime time.Time
}

// NewAPNsPusher creates a pusher from a .p8 key file.
func NewAPNsPusher(p8Path, keyID, teamID, topic string, prod bool) (*APNsPusher, error) {
	data, err := os.ReadFile(p8Path)
	if err != nil {
		return nil, fmt.Errorf("read p8 key: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", p8Path)
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse p8 key: %w", err)
	}

	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("p8 key is not ECDSA")
	}

	return &APNsPusher{
		key:    key,
		keyID:  keyID,
		teamID: teamID,
		topic:  topic,
		prod:   prod,
	}, nil
}

// Send sends a silent push notification to the given device token.
func (p *APNsPusher) Send(deviceToken string) error {
	jwt, err := p.getToken()
	if err != nil {
		return fmt.Errorf("generate JWT: %w", err)
	}

	// Silent push payload — wakes the app in the background.
	payload := map[string]any{
		"aps": map[string]any{
			"content-available": 1,
		},
	}
	body, _ := json.Marshal(payload)

	host := "https://api.sandbox.push.apple.com"
	if p.prod {
		host = "https://api.push.apple.com"
	}
	url := fmt.Sprintf("%s/3/device/%s", host, deviceToken)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", p.topic)
	req.Header.Set("apns-push-type", "background")
	req.Header.Set("apns-priority", "5") // silent pushes must use priority 5

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("APNs request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("APNs %d: %s", resp.StatusCode, string(respBody))
}

// getToken returns a cached JWT, refreshing every 50 minutes (APNs tokens expire after 60).
func (p *APNsPusher) getToken() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token != "" && time.Since(p.tokenTime) < 50*time.Minute {
		return p.token, nil
	}

	now := time.Now()
	header := base64Encode([]byte(`{"alg":"ES256","kid":"` + p.keyID + `"}`))
	claims := base64Encode([]byte(fmt.Sprintf(`{"iss":"%s","iat":%d}`, p.teamID, now.Unix())))
	unsigned := header + "." + claims

	hash := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, p.key, hash[:])
	if err != nil {
		return "", err
	}

	// ES256 signature: r and s as 32-byte big-endian, concatenated.
	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)

	p.token = unsigned + "." + base64Encode(sig)
	p.tokenTime = now
	return p.token, nil
}

func base64Encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// FCMPusher sends push notifications via Firebase Cloud Messaging (placeholder).
type FCMPusher struct {
	// TODO: implement when Android support is added.
}

// Pusher dispatches push notifications based on platform.
type Pusher struct {
	apns *APNsPusher
}

// NewPusher creates a Pusher. apns may be nil if APNs is not configured.
func NewPusher(apns *APNsPusher) *Pusher {
	return &Pusher{apns: apns}
}

// Notify sends a silent push to the given registration.
func (p *Pusher) Notify(reg Registration) error {
	switch reg.Platform {
	case "ios":
		if p.apns == nil {
			return fmt.Errorf("APNs not configured")
		}
		return p.apns.Send(reg.Token)
	case "android":
		return fmt.Errorf("Android push not yet implemented")
	default:
		return fmt.Errorf("unknown platform: %s", reg.Platform)
	}
}

// ecdsa Sign helper that handles the R and S values.
func ecdsaSignatureSize() int { return 64 }

// bigIntTo32Bytes pads a big.Int to exactly 32 bytes.
func bigIntTo32Bytes(i *big.Int) []byte {
	b := i.Bytes()
	if len(b) >= 32 {
		return b[len(b)-32:]
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}
