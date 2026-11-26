package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DBPath       string
	MemePath     string
	CookieSecure bool
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
	return c, nil
}
