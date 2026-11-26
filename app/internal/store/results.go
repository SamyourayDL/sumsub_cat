package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// VerificationEvent contains only validated, normalized routing/result fields.
// Neither raw payloads nor document details are persisted.
type VerificationEvent struct {
	ExternalID, ApplicantID, Type, Status string
	Millis                                int64
}

func (s *Store) ApplyVerification(ctx context.Context, e VerificationEvent) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var id int64
	var applicant, fingerprint sql.NullString
	var millis sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT id, sumsub_applicant_id, verification_event_ms, verification_fingerprint FROM users WHERE sumsub_external_id=?`, e.ExternalID).Scan(&id, &applicant, &millis, &fingerprint); errors.Is(err, sql.ErrNoRows) {
		return "unknown_user", nil
	} else if err != nil {
		return "", err
	}
	if applicant.Valid && applicant.String != e.ApplicantID {
		return "applicant_mismatch", nil
	}
	data, _ := json.Marshal(e)
	digest := sha256.Sum256(data)
	fp := hex.EncodeToString(digest[:])
	if millis.Valid && e.Millis < millis.Int64 {
		return "stale", nil
	}
	if millis.Valid && e.Millis == millis.Int64 && fp == fingerprint.String {
		return "duplicate", nil
	}
	status, conflict, outcome := e.Status, false, "applied"
	if millis.Valid && e.Millis == millis.Int64 {
		status, conflict, outcome = "conflict", true, "conflict"
		// Keep the first fingerprint: replaying either side cannot clear conflict.
		fp = fingerprint.String
	}
	_, err = tx.ExecContext(ctx, `UPDATE users SET verification_status=?, sumsub_applicant_id=?, verification_event_ms=?, verification_fingerprint=?, verification_conflict=? WHERE id=?`, status, e.ApplicantID, e.Millis, fp, conflict, id)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return outcome, nil
}
