// Package store persists users and opaque session digests in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
)

var ErrLoginTaken = errors.New("login already exists")

type User struct {
	ID           int64
	Login        string
	PasswordHash string
	Status       string
}

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	// Create the file privately before SQLite opens it. Existing permissions are preserved.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open database file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return s, nil
}

func (s *Store) initialize() error {
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;"); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 2 {
		return fmt.Errorf("unsupported schema version %d (maximum 2)", version)
	}
	if version == 0 {
		_, err = tx.ExecContext(ctx, `CREATE TABLE users (
   id INTEGER PRIMARY KEY AUTOINCREMENT,
   login TEXT NOT NULL UNIQUE,
   password_hash TEXT NOT NULL,
   verification_status TEXT NOT NULL DEFAULT 'not_started',
   created_at INTEGER NOT NULL
  );
  CREATE TABLE sessions (
   token_hash TEXT PRIMARY KEY,
   user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
   created_at INTEGER NOT NULL,
   expires_at INTEGER NOT NULL
  );
  CREATE INDEX sessions_expires_at ON sessions(expires_at);
  PRAGMA user_version = 1;`)
		if err != nil {
			return err
		}
	}
	if version < 2 {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE users ADD COLUMN sumsub_external_id TEXT;
   CREATE UNIQUE INDEX users_sumsub_external_id ON users(sumsub_external_id);
   PRAGMA user_version = 2;`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) UserByLogin(ctx context.Context, login string) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, "SELECT id, login, password_hash, verification_status FROM users WHERE login = ?", login).Scan(&u.ID, &u.Login, &u.PasswordHash, &u.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func (s *Store) SessionUser(ctx context.Context, digest string, now time.Time) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT u.id, u.login, u.verification_status FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > ?`, digest, now.Unix()).Scan(&u.ID, &u.Login, &u.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

// CreateSession atomically creates an optional new user, revokes the browser's
// previous session, and inserts the new session. A failed insert rolls it all back.
func (s *Store) CreateSession(ctx context.Context, user *User, register bool, digest, oldDigest string, now, expires time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if register {
		result, err := tx.ExecContext(ctx, "INSERT INTO users(login, password_hash, created_at) VALUES (?, ?, ?)", user.Login, user.PasswordHash, now.Unix())
		if err != nil {
			var se *sqlite.Error
			if errors.As(err, &se) && se.Code() == 2067 {
				return ErrLoginTaken
			}
			return err
		}
		user.ID, err = result.LastInsertId()
		if err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ? OR expires_at <= ?", oldDigest, now.Unix()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO sessions(token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)", digest, user.ID, now.Unix(), expires.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteSession(ctx context.Context, digest string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", digest)
	return err
}
