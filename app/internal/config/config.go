package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DBPath          string
	MemePath        string
	CookieSecure    bool
	TrustFlyProxy   bool
	SumsubAppToken  string
	SumsubSecretKey string
	SumsubLevelName string
}

func Load() (Config, error) {
	c := Config{DBPath: "data/app.db", MemePath: "resources/meme.jpg"}
	if value, ok := os.LookupEnv("DB_PATH"); ok {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "?\x00") || strings.HasPrefix(value, "file:") || value == ":memory:" {
			return c, fmt.Errorf("DB_PATH must be a nonempty filesystem path, not a SQLite URI")
		}
		c.DBPath = value
	}
	if value, ok := os.LookupEnv("MEME_PATH"); ok {
		if strings.TrimSpace(value) == "" || strings.ContainsRune(value, '\x00') {
			return c, fmt.Errorf("MEME_PATH must be a nonempty filesystem path")
		}
		c.MemePath = value
	}
	if value, ok := os.LookupEnv("COOKIE_SECURE"); ok {
		var err error
		c.CookieSecure, err = strconv.ParseBool(value)
		if err != nil {
			return c, fmt.Errorf("COOKIE_SECURE must be true or false")
		}
	}
	if value, ok := os.LookupEnv("TRUST_FLY_PROXY"); ok {
		var err error
		c.TrustFlyProxy, err = strconv.ParseBool(value)
		if err != nil {
			return c, fmt.Errorf("TRUST_FLY_PROXY must be true or false")
		}
	}
	c.SumsubAppToken = os.Getenv("SUMSUB_APP_TOKEN")
	c.SumsubSecretKey = os.Getenv("SUMSUB_SECRET_KEY")
	c.SumsubLevelName = os.Getenv("SUMSUB_LEVEL_NAME")
	values := []string{c.SumsubAppToken, c.SumsubSecretKey, c.SumsubLevelName}
	configured := 0
	for _, v := range values {
		if v != "" {
			configured++
		}
	}
	if configured != 0 {
		for _, v := range values {
			if strings.TrimSpace(v) == "" || strings.TrimSpace(v) != v || strings.ContainsAny(v, "\r\n\x00") {
				return c, fmt.Errorf("SUMSUB_APP_TOKEN, SUMSUB_SECRET_KEY and SUMSUB_LEVEL_NAME must all be set without surrounding whitespace or control characters")
			}
		}
	}
	return c, nil
}
