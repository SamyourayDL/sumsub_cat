package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DBPath       string
	CookieSecure bool
}

func Load() (Config, error) {
	c := Config{DBPath: "data/app.db"}
	if value, ok := os.LookupEnv("DB_PATH"); ok {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "?\x00") || strings.HasPrefix(value, "file:") || value == ":memory:" {
			return c, fmt.Errorf("DB_PATH must be a nonempty filesystem path, not a SQLite URI")
		}
		c.DBPath = value
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
