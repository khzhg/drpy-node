package signer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

type Signer struct {
	secret     string
	ttlSeconds int
	enabled    bool
}

func New(secret string, ttlSeconds int, enabled bool) *Signer {
	return &Signer{
		secret:     secret,
		ttlSeconds: ttlSeconds,
		enabled:    enabled,
	}
}

func (s *Signer) IsEnabled() bool {
	return s.enabled
}

func (s *Signer) Generate(url string) (string, string, error) {
	if !s.enabled {
		return "", "", fmt.Errorf("signing is disabled")
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := url + "|" + ts
	
	h := hmac.New(sha256.New, []byte(s.secret))
	h.Write([]byte(payload))
	sign := hex.EncodeToString(h.Sum(nil))
	
	return sign, ts, nil
}

func (s *Signer) Verify(url, sign, ts string) error {
	if !s.enabled {
		return nil // Always pass if signing is disabled
	}

	if sign == "" || ts == "" {
		return fmt.Errorf("missing signature parameters")
	}

	// Parse timestamp
	timestamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}

	// Check if not expired
	now := time.Now().Unix()
	if now-timestamp > int64(s.ttlSeconds) {
		return fmt.Errorf("signature expired")
	}

	// Generate expected signature
	payload := url + "|" + ts
	h := hmac.New(sha256.New, []byte(s.secret))
	h.Write([]byte(payload))
	expectedSign := hex.EncodeToString(h.Sum(nil))

	if sign != expectedSign {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}