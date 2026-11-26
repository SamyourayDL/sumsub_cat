package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConcurrentResultsAndWriteFailure(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "results.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	u := &User{Login: "alice", PasswordHash: "hash"}
	if err = db.CreateSession(ctx, u, true, "session", "", time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	id, err := db.VerificationID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.Exec(`CREATE TRIGGER fail_result BEFORE UPDATE OF verification_status ON users BEGIN SELECT RAISE(ABORT, 'test storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	e := VerificationEvent{ExternalID: id, ApplicantID: "applicant", Type: "applicantReviewed", Status: "approved", Millis: 1000}
	if _, err = db.ApplyVerification(ctx, e); err == nil {
		t.Fatal("write error accepted")
	}
	var applicant *string
	db.db.QueryRow("SELECT sumsub_applicant_id FROM users").Scan(&applicant)
	if applicant != nil {
		t.Fatal("partial write")
	}
	if _, err = db.db.Exec("DROP TRIGGER fail_result"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ApplyVerification(ctx, e); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := int64(1); i <= 30; i++ {
		wg.Add(1)
		go func(i int64) {
			defer wg.Done()
			next := e
			next.Millis += i
			next.Status = "pending"
			if i == 30 {
				next.Status = "approved"
			}
			if _, err := db.ApplyVerification(ctx, next); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	user, err := db.UserByLogin(ctx, "alice")
	if err != nil || user.Status != "approved" {
		t.Fatal("newest event lost")
	}
	for _, status := range []string{"approved", "rejected"} {
		wg.Add(1)
		go func(status string) {
			defer wg.Done()
			next := e
			next.Millis = 2000
			next.Status = status
			if _, err := db.ApplyVerification(ctx, next); err != nil {
				t.Error(err)
			}
		}(status)
	}
	wg.Wait()
	user, err = db.UserByLogin(ctx, "alice")
	if err != nil || user.Status != "conflict" {
		t.Fatal("concurrent conflict missed")
	}
}
