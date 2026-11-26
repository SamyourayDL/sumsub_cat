package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchemaAndAtomicSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "app.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, p := range []string{path, filepath.Dir(path)} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatal("database permissions too broad")
		}
	}
	var fk int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatal("foreign keys disabled")
	}
	now := time.Now()
	ctx := context.Background()
	u := &User{Login: "alice", PasswordHash: "test-hash"}
	if err := s.CreateSession(ctx, u, true, "digest", "", now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	v := &User{Login: "bob", PasswordHash: "test-hash"}
	if err := s.CreateSession(ctx, v, true, "digest", "", now, now.Add(time.Hour)); err == nil {
		t.Fatal("expected duplicate token error")
	}
	found, err := s.UserByLogin(ctx, "bob")
	if err != nil || found != nil {
		t.Fatal("partial user committed")
	}
	if err := s.CreateSession(ctx, u, false, "new-digest", "digest", now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	found, err = s.SessionUser(ctx, "digest", now)
	if err != nil || found != nil {
		t.Fatal("old session not revoked")
	}
	if _, err := s.db.Exec("UPDATE sessions SET expires_at=0"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("expired sessions not cleared")
	}
	if _, err := s.db.Exec("PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if unexpected, err := Open(path); err == nil {
		unexpected.Close()
		t.Fatal("accepted newer schema")
	}
}
