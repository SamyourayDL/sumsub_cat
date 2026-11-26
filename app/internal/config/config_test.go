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

func TestMemePath(t *testing.T) {
	t.Setenv("DB_PATH", "data/test.db")
	t.Setenv("COOKIE_SECURE", "false")
	for _, value := range []string{"", "  "} {
		t.Setenv("MEME_PATH", value)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	t.Setenv("MEME_PATH", "/tmp/cat.png")
	c, err := Load()
	if err != nil || c.MemePath != "/tmp/cat.png" {
		t.Fatalf("config: %+v %v", c, err)
	}
}
