package config

import "testing"

func TestConfig(t *testing.T) {
	for _, value := range []string{"", ":memory:", "file:test.db", "db?mode=memory"} {
		t.Setenv("DB_PATH", value)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted path %q", value)
		}
	}
	t.Setenv("DB_PATH", "data/demo.db")
	t.Setenv("COOKIE_SECURE", "invalid")
	if _, err := Load(); err == nil {
		t.Fatal("accepted invalid bool")
	}
	for _, value := range []string{"true", "false"} {
		t.Setenv("COOKIE_SECURE", value)
		c, err := Load()
		if err != nil || c.CookieSecure != (value == "true") {
			t.Fatal("config", err)
		}
	}
}
