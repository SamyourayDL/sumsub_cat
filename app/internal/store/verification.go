package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// VerificationID allocates once, including when several tabs start concurrently.
// No login or other personal identifier is sent to the provider.
func (s *Store) VerificationID(ctx context.Context, userID int64) (string, error) {
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	candidate := "demo-" + hex.EncodeToString(random[:])
	var id string
	err := s.db.QueryRowContext(ctx, `UPDATE users SET sumsub_external_id = COALESCE(sumsub_external_id, ?)
  WHERE id = ? RETURNING sumsub_external_id`, candidate, userID).Scan(&id)
	return id, err
}
