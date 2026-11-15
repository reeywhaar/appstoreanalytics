package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// How long a session lasts and how often its window is pushed forward.
//
// The throttle is not an optimization: without it every request rewrites the row and emits a
// Set-Cookie, for a window measured in days.
const (
	SessionLifetime = 30 * 24 * time.Hour
	SessionRefresh  = time.Hour
)

// userAgentMax is how much of the browser's description of itself is kept.
const userAgentMax = 400

// Session is one live sign-in.
//
// There is no field holding the cookie value and no way to get one from this package. The
// table is keyed by sha256 of it, so nothing readable ever contains something replayable.
type Session struct {
	UserAgent  string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// Secret is 32 random bytes, for a cookie value.
func Secret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// CreateSession records a sign-in. token is the cookie value; only its hash is stored.
func (s *Store) CreateSession(ctx context.Context, token, userAgent string) (*Session, error) {
	now := s.Now()
	expires := now.Add(SessionLifetime)
	userAgent = truncate(userAgent, userAgentMax)
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO sessions (id_hash, user_agent, created_at, last_seen_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		hashToken(token), userAgent, unix(now), unix(now), unix(expires))
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &Session{UserAgent: userAgent, CreatedAt: now, LastSeenAt: now, ExpiresAt: expires}, nil
}

// SessionByToken returns a live session, or ErrNotFound. Expiry is applied in the query.
func (s *Store) SessionByToken(ctx context.Context, token string) (*Session, error) {
	var (
		sess                       Session
		created, lastSeen, expires int64
	)
	err := s.reader.QueryRowContext(ctx,
		`SELECT user_agent, created_at, last_seen_at, expires_at
		   FROM sessions WHERE id_hash = ? AND expires_at > ?`,
		hashToken(token), unix(s.Now())).
		Scan(&sess.UserAgent, &created, &lastSeen, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	sess.CreatedAt = fromUnix(created)
	sess.LastSeenAt = fromUnix(lastSeen)
	sess.ExpiresAt = fromUnix(expires)
	return &sess, nil
}

// TouchSession slides the window forward and reports whether it moved.
func (s *Store) TouchSession(ctx context.Context, token, userAgent string) (time.Time, bool, error) {
	now := s.Now()
	expires := now.Add(SessionLifetime)
	res, err := s.writer.ExecContext(ctx,
		`UPDATE sessions SET last_seen_at = ?, expires_at = ?, user_agent = ?
		   WHERE id_hash = ? AND last_seen_at <= ?`,
		unix(now), unix(expires), truncate(userAgent, userAgentMax),
		hashToken(token), unix(now.Add(-SessionRefresh)))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("touch session: %w", err)
	}
	n, _ := res.RowsAffected()
	return expires, n > 0, nil
}

// DeleteSession signs one session out. One already gone is not an error: a sign-out is a
// statement about the future.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.writer.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, hashToken(token))
	return err
}

// SweepSessions collects lapsed rows, which are already unusable.
func (s *Store) SweepSessions(ctx context.Context) (int64, error) {
	res, err := s.writer.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, unix(s.Now()))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// bcryptCost is a variable so a test suite need not spend a second on every store it opens.
var bcryptCost = 12

// SetBcryptCost is for tests.
func SetBcryptCost(cost int) { bcryptCost = cost }

const credentialKey = "credential"

// KeepCredential remembers the sign-in the environment names, and signs every browser out when
// it is not the one remembered: a password changed because it leaked has to end the sessions
// made with it. It reports whether it did.
//
// What is remembered is a bcrypt hash, never the password.
func (s *Store) KeepCredential(ctx context.Context, username, password string) (bool, error) {
	secret := credentialSecret(username, password)
	var kept string
	err := s.reader.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, credentialKey).Scan(&kept)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read the credential: %w", err)
	}
	if kept != "" && bcrypt.CompareHashAndPassword([]byte(kept), secret) == nil {
		return false, nil
	}
	hash, err := bcrypt.GenerateFromPassword(secret, bcryptCost)
	if err != nil {
		return false, err
	}
	var signedOut bool
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM sessions`)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		signedOut = n > 0
		_, err = tx.ExecContext(ctx,
			`INSERT INTO meta (key, value) VALUES (?, ?)
			 ON CONFLICT (key) DO UPDATE SET value = excluded.value`, credentialKey, string(hash))
		return err
	})
	if err != nil {
		return false, fmt.Errorf("keep the credential: %w", err)
	}
	return signedOut, nil
}

// credentialSecret is what bcrypt is given: a digest, because bcrypt reads only 72 bytes and a
// username and a password together can be longer.
func credentialSecret(username, password string) []byte {
	sum := sha256.Sum256([]byte(username + "\x00" + password))
	return []byte(base64.RawStdEncoding.EncodeToString(sum[:]))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
