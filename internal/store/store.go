package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"golang.org/x/crypto/bcrypt"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
}

type Session struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) EnsureBootstrapAdmin(ctx context.Context, username string, password string) error {
	if username == "" || password == "" {
		return nil
	}

	count := 0
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	_, err := s.CreateUser(ctx, username, password, RoleAdmin)
	return err
}

func (s *Store) CreateUser(ctx context.Context, username string, password string, role Role) (User, error) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		username,
		string(hashBytes),
		role,
		now,
	)
	if err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("last insert id: %w", err)
	}

	return User{
		ID:           id,
		Username:     username,
		PasswordHash: string(hashBytes),
		Role:         role,
		CreatedAt:    now,
	}, nil
}

func (s *Store) AuthenticateUser(ctx context.Context, username string, password string) (User, error) {
	user, err := s.UserByUsername(ctx, username)
	if err != nil {
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return User{}, errors.New("invalid credentials")
	}

	return user, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, errors.New("invalid credentials")
		}
		return User{}, fmt.Errorf("query user: %w", err)
	}

	return user, nil
}

func (s *Store) UserBySessionToken(ctx context.Context, token string) (User, error) {
	var user User
	err := s.db.QueryRowContext(
		ctx,
		`SELECT u.id, u.username, u.password_hash, u.role, u.created_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = ? AND s.expires_at > ?`,
		hashToken(token),
		time.Now().UTC(),
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, errors.New("session not found")
		}
		return User{}, fmt.Errorf("query session user: %w", err)
	}
	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (Session, error) {
	token, err := newToken()
	if err != nil {
		return Session{}, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		userID,
		hashToken(token),
		expiresAt,
		now,
	)
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Session{}, fmt.Errorf("session id: %w", err)
	}

	return Session{
		ID:        id,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) WriteAuditLog(ctx context.Context, actorUserID *int64, action string, targetType string, targetID string, metadataJSON string) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, metadata_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		actorUserID,
		action,
		targetType,
		targetID,
		metadataJSON,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_user_id INTEGER NULL,
			action TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			metadata_json TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY(actor_user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate statement: %w", err)
		}
	}

	return nil
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
