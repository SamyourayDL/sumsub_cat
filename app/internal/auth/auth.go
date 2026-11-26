package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"sumsub-demo/internal/store"
)

const SessionLifetime = 24 * time.Hour
const PasswordCost = 12

var ErrCredentials = errors.New("Неверный логин или пароль")
var loginPattern = regexp.MustCompile(`^[a-z0-9_-]{3,32}$`)

type Service struct {
	Store     *store.Store
	dummyHash []byte
}

func New(s *store.Store) (*Service, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-comparison"), PasswordCost)
	if err != nil {
		return nil, err
	}
	return &Service{Store: s, dummyHash: hash}, nil
}

func NormalizeLogin(login string) string { return strings.ToLower(strings.TrimSpace(login)) }
func Validate(login, password string) error {
	if !loginPattern.MatchString(login) {
		return errors.New("Логин: 3–32 латинские буквы, цифры, дефис или подчёркивание")
	}
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || len(password) > 72 {
		return errors.New("Пароль: минимум 12 символов и максимум 72 байта UTF-8")
	}
	return nil
}
func Digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) CurrentUser(ctx context.Context, token string) (*store.User, error) {
	if len(token) != 64 {
		return nil, nil
	}
	if _, err := hex.DecodeString(token); err != nil {
		return nil, nil
	}
	return s.Store.SessionUser(ctx, Digest(token), time.Now())
}

func (s *Service) Authenticate(ctx context.Context, login, password, oldToken string, register bool) (string, time.Time, error) {
	login = NormalizeLogin(login)
	var user *store.User
	if register {
		if err := Validate(login, password); err != nil {
			return "", time.Time{}, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
		if err != nil {
			return "", time.Time{}, err
		}
		user = &store.User{Login: login, PasswordHash: string(hash)}
	} else {
		var err error
		user, err = s.Store.UserByLogin(ctx, login)
		if err != nil {
			return "", time.Time{}, err
		}
		hash := s.dummyHash
		if user != nil {
			hash = []byte(user.PasswordHash)
		}
		// Avoid bcrypt's early return for an oversized password while still comparing.
		candidate := []byte(password)
		if len(candidate) > 72 {
			candidate = []byte("invalid-oversized-password")
		}
		err = bcrypt.CompareHashAndPassword(hash, candidate)
		if user == nil || err != nil || Validate(login, password) != nil {
			return "", time.Time{}, ErrCredentials
		}
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", time.Time{}, err
	}
	raw := hex.EncodeToString(token)
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(SessionLifetime)
	if err := s.Store.CreateSession(ctx, user, register, Digest(raw), Digest(oldToken), now, expires); err != nil {
		return "", time.Time{}, err
	}
	return raw, expires, nil
}
