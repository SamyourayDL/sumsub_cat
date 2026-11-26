package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"sumsub-demo/internal/store"
)

type webhookPayload struct {
	Type          string `json:"type"`
	ApplicantID   string `json:"applicantId"`
	ApplicantType string `json:"applicantType"`
	ExternalID    string `json:"externalUserId"`
	Level         string `json:"levelName"`
	Sandbox       *bool  `json:"sandboxMode"`
	Test          bool   `json:"testMode"`
	CreatedAtMs   string `json:"createdAtMs"`
	CreatedAt     string `json:"createdAt"`
	Review        struct {
		Answer string `json:"reviewAnswer"`
		Reject string `json:"reviewRejectType"`
	} `json:"reviewResult"`
}

func (h *handler) webhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookSecret == "" || h.levelName == "" {
		http.Error(w, "Webhook unavailable", 503)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			http.Error(w, "Body too large", 413)
		} else {
			http.Error(w, "Invalid body", 400)
		}
		return
	}
	alg, sig := r.Header.Values("X-Payload-Digest-Alg"), r.Header.Values("X-Payload-Digest")
	if len(alg) != 1 || alg[0] != "HMAC_SHA256_HEX" || len(sig) != 1 {
		http.Error(w, "Invalid signature", 403)
		return
	}
	digest, err := hex.DecodeString(sig[0])
	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write(body)
	if err != nil || !hmac.Equal(digest, mac.Sum(nil)) {
		http.Error(w, "Invalid signature", 403)
		return
	}
	var p webhookPayload
	if json.Unmarshal(body, &p) != nil || p.Type == "" {
		http.Error(w, "Invalid event", 400)
		return
	}
	if p.Test {
		w.WriteHeader(200)
		return
	}
	status := ""
	switch p.Type {
	case "applicantCreated":
		status = "in_progress"
	case "applicantPending":
		status = "pending"
	case "applicantOnHold":
		status = "on_hold"
	case "applicantReset":
		status = "not_started"
	case "applicantReviewed":
	default:
		w.WriteHeader(200)
		return
	}
	if p.Level == "" || p.Sandbox == nil || p.ApplicantType == "" || strings.TrimSpace(p.ExternalID) == "" || strings.TrimSpace(p.ApplicantID) == "" {
		http.Error(w, "Missing event fields", 400)
		return
	}
	if p.Level != h.levelName || !*p.Sandbox || p.ApplicantType != "individual" {
		w.WriteHeader(200)
		return
	}
	if p.Type == "applicantReviewed" {
		switch {
		case p.Review.Answer == "GREEN" && p.Review.Reject == "":
			status = "approved"
		case p.Review.Answer == "RED" && p.Review.Reject == "RETRY":
			status = "retry"
		case p.Review.Answer == "RED" && p.Review.Reject == "FINAL":
			status = "rejected"
		default:
			http.Error(w, "Invalid review result", 400)
			return
		}
	}
	value, layout := p.CreatedAtMs, "2006-01-02 15:04:05.000"
	if value == "" {
		value, layout = p.CreatedAt, "2006-01-02 15:04:05"
	}
	at, err := time.Parse(layout, value)
	if err != nil || at.Format(layout) != value {
		http.Error(w, "Invalid event time", 400)
		return
	}
	outcome, err := h.auth.Store.ApplyVerification(r.Context(), store.VerificationEvent{ExternalID: p.ExternalID, ApplicantID: p.ApplicantID, Type: p.Type, Status: status, Millis: at.UnixMilli()})
	if err != nil {
		log.Print("sumsub webhook: storage_error")
		serverError(w)
		return
	}
	log.Printf("sumsub webhook: %s", outcome)
	w.WriteHeader(200)
}
