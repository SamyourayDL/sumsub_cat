// Package sumsub implements the server-only SDK token exchange.
package sumsub

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

const tokenPath = "/resources/accessTokens/sdk"
const TokenTTL = 600

var ErrUnavailable = errors.New("verification provider unavailable")

type Client struct {
	appToken, secret, level, baseURL string
	http                             *http.Client
	now                              func() time.Time
}

func New(appToken, secret, level string) *Client {
	return &Client{appToken: appToken, secret: secret, level: level,
		baseURL: "https://api.sumsub.com", now: time.Now,
		http: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) Token(ctx context.Context, userID string) (string, error) {
	body, err := json.Marshal(struct {
		UserID string `json:"userId"`
		Level  string `json:"levelName"`
		TTL    int    `json:"ttlInSecs"`
	}{userID, c.level, TokenTTL})
	if err != nil {
		return "", ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+tokenPath, bytes.NewReader(body))
	if err != nil {
		return "", ErrUnavailable
	}
	timestamp := strconv.FormatInt(c.now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write([]byte(timestamp + http.MethodPost + tokenPath))
	mac.Write(body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Token", c.appToken)
	req.Header.Set("X-App-Access-Ts", timestamp)
	req.Header.Set("X-App-Access-Sig", hex.EncodeToString(mac.Sum(nil)))
	resp, err := c.http.Do(req)
	if err != nil {
		return "", ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024+1))
	if err != nil || len(raw) > 16*1024 {
		return "", ErrUnavailable
	}
	var result struct {
		Token  string `json:"token"`
		UserID string `json:"userId"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Token == "" || len(result.Token) > 1024 || result.UserID != userID {
		return "", ErrUnavailable
	}
	// The token is opaque; never parse it, persist it or include it in an error.
	return result.Token, nil
}
