package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestVerificationMigrationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY AUTOINCREMENT,login TEXT NOT NULL UNIQUE,password_hash TEXT NOT NULL,verification_status TEXT NOT NULL DEFAULT 'not_started',created_at INTEGER NOT NULL);
 CREATE TABLE sessions(token_hash TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL);
 INSERT INTO users(login,password_hash,created_at) VALUES('existing','hash',1);
 INSERT INTO sessions VALUES('existing-session',1,1,4102444800);
 PRAGMA user_version=1;`)
	if err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	user, err := db.SessionUser(ctx, "existing-session", time.Now())
	if err != nil || user == nil || user.Login != "existing" {
		t.Fatal("migration lost session")
	}
	const n = 20
	ids := make(chan string, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := db.VerificationID(ctx, 1)
			if err != nil {
				t.Error(err)
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if len(id) != 69 || id != first {
			t.Fatal("concurrent allocation diverged")
		}
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := db.VerificationID(ctx, 1)
	if err != nil || id != first {
		t.Fatal("id lost on reopen")
	}
	if _, err = db.VerificationID(ctx, 999); err == nil {
		t.Fatal("missing user accepted")
	}
	if _, err = db.db.Exec("DELETE FROM sessions; DELETE FROM users; INSERT INTO users(login,password_hash,created_at) VALUES('existing','hash',1)"); err != nil {
		t.Fatal(err)
	}
	u, err := db.UserByLogin(ctx, "existing")
	if err != nil {
		t.Fatal(err)
	}
	next, err := db.VerificationID(ctx, u.ID)
	if err != nil || next == first {
		t.Fatal("recreated user reused id")
	}
	if _, err = db.db.Exec("INSERT INTO users(login,password_hash,created_at,sumsub_external_id) VALUES('duplicate','hash',1,?)", next); err == nil {
		t.Fatal("duplicate external id accepted")
	}
}
