package config

import (
	"strings"
	"testing"
)

func TestSumsubConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, token, secret, level string
		valid                      bool
	}{
		{"disabled", "", "", "", true}, {"configured", "app-test", "secret-test", "id-and-liveness", true},
		{"missing-secret", "app-test", "", "level", false}, {"only-level", "", "", "level", false},
		{"blank", " ", "secret-test", "level", false}, {"newline", "app\n", "secret-test", "level", false},
		{"surrounding-space", "app-test", " secret-test", "level", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SUMSUB_APP_TOKEN", tc.token)
			t.Setenv("SUMSUB_SECRET_KEY", tc.secret)
			t.Setenv("SUMSUB_LEVEL_NAME", tc.level)
			cfg, err := Load()
			if (err == nil) != tc.valid {
				t.Fatal("validation mismatch")
			}
			if err != nil && strings.Contains(err.Error(), "secret-test") {
				t.Fatal("secret leaked")
			}
			if tc.valid && cfg.SumsubLevelName != tc.level {
				t.Fatal("level changed")
			}
		})
	}
}

func TestWebhookSecretConfiguration(t *testing.T) {
	for _, secret := range []string{"", "local-webhook-secret", " invalid", "secret\n"} {
		t.Run(secret, func(t *testing.T) {
			t.Setenv("SUMSUB_APP_TOKEN", "")
			t.Setenv("SUMSUB_SECRET_KEY", "")
			t.Setenv("SUMSUB_LEVEL_NAME", "")
			t.Setenv("SUMSUB_WEBHOOK_SECRET", secret)
			cfg, err := Load()
			valid := secret == "" || secret == "local-webhook-secret"
			if (err == nil) != valid {
				t.Fatal("validation")
			}
			if valid && cfg.SumsubWebhookSecret != secret {
				t.Fatal("missing webhook secret")
			}
			if err != nil && strings.Contains(err.Error(), secret) {
				t.Fatal("secret leaked")
			}
		})
	}
}
