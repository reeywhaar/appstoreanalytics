package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// HooksKept is how many deliveries are kept: the newest, the rest swept as each arrives.
const HooksKept = 1000

// HookEndpoint is an address webhooks are delivered to: a label for people, the token that is the
// last part of its URL, and the secret App Store Connect signs what it sends there with.
type HookEndpoint struct {
	ID        int64
	Label     string
	Token     string
	Secret    string
	CreatedAt time.Time
	// What has arrived through it, among the deliveries kept; LastAt is zero for nothing.
	Count  int
	LastAt time.Time
}

// labelMax is how long a label can be, in characters.
const labelMax = 100

// AddHookEndpoint makes a new address, with secret, or a generated one when it is "".
func (s *Store) AddHookEndpoint(ctx context.Context, label, secret string) (*HookEndpoint, error) {
	label = strings.TrimSpace(label)
	switch {
	case label == "":
		return nil, Invalid("An address needs a label, to tell it from the others.")
	case utf8.RuneCountInString(label) > labelMax:
		return nil, Invalid("A label is at most %d characters.", labelMax)
	}
	if secret == "" {
		var err error
		if secret, err = randomText(32); err != nil {
			return nil, err
		}
	}
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	e := HookEndpoint{Label: label, Token: base64.RawURLEncoding.EncodeToString(b[:]), Secret: secret, CreatedAt: s.Now().UTC().Truncate(time.Second)}
	res, err := s.writer.ExecContext(ctx,
		`INSERT INTO hook_endpoints (label, token, secret, created_at) VALUES (?, ?, ?, ?)`,
		e.Label, e.Token, e.Secret, unix(e.CreatedAt))
	if err != nil {
		return nil, fmt.Errorf("add a hook endpoint: %w", err)
	}
	if e.ID, err = res.LastInsertId(); err != nil {
		return nil, err
	}
	s.changed()
	return &e, nil
}

// HookEndpoints is every address, the oldest first, with what has arrived through each.
func (s *Store) HookEndpoints(ctx context.Context) ([]HookEndpoint, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT e.id, e.label, e.token, e.secret, e.created_at, COUNT(h.id), MAX(h.received_at)
		   FROM hook_endpoints e LEFT JOIN hooks h ON h.endpoint_id = e.id
		  GROUP BY e.id ORDER BY e.id`)
	if err != nil {
		return nil, fmt.Errorf("hook endpoints: %w", err)
	}
	defer rows.Close()
	out := []HookEndpoint{}
	for rows.Next() {
		var (
			e       HookEndpoint
			created int64
			last    sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &e.Label, &e.Token, &e.Secret, &created, &e.Count, &last); err != nil {
			return nil, err
		}
		e.CreatedAt = fromUnix(created)
		if last.Valid {
			e.LastAt = fromUnix(last.Int64)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// HookEndpointByToken is the address a URL's token names, or ErrNotFound.
func (s *Store) HookEndpointByToken(ctx context.Context, token string) (*HookEndpoint, error) {
	return s.hookEndpoint(ctx, `token = ?`, token)
}

// HookEndpoint is one address by its id, or ErrNotFound.
func (s *Store) HookEndpoint(ctx context.Context, id int64) (*HookEndpoint, error) {
	return s.hookEndpoint(ctx, `id = ?`, id)
}

func (s *Store) hookEndpoint(ctx context.Context, where string, arg any) (*HookEndpoint, error) {
	var (
		e       HookEndpoint
		created int64
	)
	err := s.reader.QueryRowContext(ctx,
		`SELECT id, label, token, secret, created_at FROM hook_endpoints WHERE `+where, arg).
		Scan(&e.ID, &e.Label, &e.Token, &e.Secret, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, NotFound("There is no such endpoint.")
	}
	if err != nil {
		return nil, fmt.Errorf("hook endpoint: %w", err)
	}
	e.CreatedAt = fromUnix(created)
	return &e, nil
}

// RemoveHookEndpoint removes an address: it answers nothing from now. What arrived through it is
// kept, and still says its label.
func (s *Store) RemoveHookEndpoint(ctx context.Context, id int64) error {
	res, err := s.writer.ExecContext(ctx, `DELETE FROM hook_endpoints WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove a hook endpoint: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return NotFound("There is no such address.")
	}
	s.changed()
	return nil
}

// randomText is n letters and digits: pasted into any form, it needs no escaping anywhere.
func randomText(n int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	// The largest multiple of the alphabet's length under 256: a byte above it is thrown away, or
	// the first few characters would come up more often than the rest.
	const fair = 256 - 256%len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) < fair && len(out) < n {
				out = append(out, alphabet[int(b)%len(alphabet)])
			}
		}
	}
	return string(out), nil
}

// Hook is one delivery.
type Hook struct {
	ID         int64
	ReceivedAt time.Time
	// The address it came through, and that address's label then; EndpointID outlives the
	// address, and is never another's, ids not being reused.
	EndpointID  int64
	Via         string
	Event       string
	Summary     string
	ContentType string
	Body        []byte
	// ProvenBy is who it is proven to come from, "" for nobody; ProofError why it failed to be.
	ProvenBy   string
	ProofError string
	MailedAt   time.Time
	// MailedTo is whom it was mailed to, MailError why the relay did not take it to some.
	MailedTo  []string
	MailError string
}

// SaveHook keeps a delivery and sweeps all but the newest HooksKept, and says its id.
func (s *Store) SaveHook(ctx context.Context, h Hook) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO hooks (received_at, endpoint_id, via, event, summary, content_type, body, proven_by, proof_error)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			unix(s.Now()), h.EndpointID, h.Via, h.Event, h.Summary, h.ContentType, h.Body, h.ProvenBy, h.ProofError)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM hooks WHERE id <= ?`, id-HooksKept)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("save a hook: %w", err)
	}
	// What came to a webhook is nowhere else.
	s.changed()
	return id, nil
}

// MarkHookMailed records whom a delivery was mailed to and how that went: why is "" when the
// relay took every one.
func (s *Store) MarkHookMailed(ctx context.Context, id int64, to []string, why string) error {
	_, err := s.writer.ExecContext(ctx,
		`UPDATE hooks SET mailed_at = ?, mailed_to = ?, mail_error = ? WHERE id = ?`,
		unix(s.Now()), strings.Join(to, ", "), why, id)
	if err == nil {
		s.changed()
	}
	return err
}

// Hooks is the newest deliveries, without their bodies.
func (s *Store) Hooks(ctx context.Context, limit int) ([]Hook, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT id, received_at, COALESCE(endpoint_id, 0), via, event, summary, content_type, length(body), proven_by, proof_error, mailed_at, mailed_to, mail_error
		   FROM hooks ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("hooks: %w", err)
	}
	defer rows.Close()
	out := []Hook{}
	for rows.Next() {
		var (
			h        Hook
			received int64
			size     int
			mailed   sql.NullInt64
		)
		var to string
		if err := rows.Scan(&h.ID, &received, &h.EndpointID, &h.Via, &h.Event, &h.Summary, &h.ContentType, &size, &h.ProvenBy, &h.ProofError, &mailed, &to, &h.MailError); err != nil {
			return nil, err
		}
		h.MailedTo = recipients(to)
		h.ReceivedAt = fromUnix(received)
		if mailed.Valid {
			h.MailedAt = fromUnix(mailed.Int64)
		}
		h.Body = make([]byte, size)
		out = append(out, h)
	}
	return out, rows.Err()
}

// Hook is one delivery with its body, or ErrNotFound.
func (s *Store) Hook(ctx context.Context, id int64) (*Hook, error) {
	var (
		h        Hook
		received int64
		mailed   sql.NullInt64
		to       string
	)
	err := s.reader.QueryRowContext(ctx,
		`SELECT id, received_at, COALESCE(endpoint_id, 0), via, event, summary, content_type, body, proven_by, proof_error, mailed_at, mailed_to, mail_error
		   FROM hooks WHERE id = ?`, id).
		Scan(&h.ID, &received, &h.EndpointID, &h.Via, &h.Event, &h.Summary, &h.ContentType, &h.Body, &h.ProvenBy, &h.ProofError, &mailed, &to, &h.MailError)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, NotFound("There is no such delivery; only the newest %d are kept.", HooksKept)
	}
	if err != nil {
		return nil, fmt.Errorf("hook: %w", err)
	}
	h.ReceivedAt = fromUnix(received)
	if mailed.Valid {
		h.MailedAt = fromUnix(mailed.Int64)
	}
	h.MailedTo = recipients(to)
	return &h, nil
}

// recipients reads a delivery's mailed_to.
func recipients(to string) []string {
	if to == "" {
		return []string{}
	}
	return strings.Split(to, ", ")
}

// HookEvent is a kind of event that has arrived: how often, and when last.
type HookEvent struct {
	Event  string `json:"event"`
	Count  int    `json:"count"`
	LastAt int64  `json:"last_at"`
}

// HookEvents is every kind of event among the deliveries kept, the most recent first.
func (s *Store) HookEvents(ctx context.Context) ([]HookEvent, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT event, COUNT(*), MAX(received_at) FROM hooks GROUP BY event ORDER BY 3 DESC`)
	if err != nil {
		return nil, fmt.Errorf("hook events: %w", err)
	}
	defer rows.Close()
	out := []HookEvent{}
	for rows.Next() {
		var e HookEvent
		if err := rows.Scan(&e.Event, &e.Count, &e.LastAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// HookMail is somebody mailed of webhooks, the kinds of event they are mailed of, and the
// addresses they are mailed from.
type HookMail struct {
	ID     int64    `json:"id"`
	To     string   `json:"to"`
	Events []string `json:"events"`
	// Endpoints are the addresses, by id; none is every address, those added later too. An
	// address removed stays named and matches nothing, rather than its going leaving none, which
	// would be every address.
	Endpoints []int64 `json:"endpoints"`
}

// From reports whether a delivery through an address is one to mail.
func (m HookMail) From(endpoint int64) bool {
	return len(m.Endpoints) == 0 || slices.Contains(m.Endpoints, endpoint)
}

// Mails reports whether an event is one to mail. A type chosen covers its subtypes: SUBSCRIBED
// mails SUBSCRIBED/INITIAL_BUY and SUBSCRIBED/RESUBSCRIBE.
func (m HookMail) Mails(event string) bool {
	if m.To == "" {
		return false
	}
	for _, e := range m.Events {
		if e == event || strings.HasPrefix(event, e+"/") {
			return true
		}
	}
	return false
}

// HookMails is everybody mailed of webhooks, the first added first.
func (s *Store) HookMails(ctx context.Context) ([]HookMail, error) {
	rows, err := s.reader.QueryContext(ctx, `SELECT id, to_address, events, endpoints FROM hook_mails ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("hook mails: %w", err)
	}
	defer rows.Close()
	out := []HookMail{}
	for rows.Next() {
		var (
			m         HookMail
			raw, from string
		)
		if err := rows.Scan(&m.ID, &m.To, &raw, &from); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &m.Events); err != nil {
			return nil, fmt.Errorf("hook mail %d's events: %w", m.ID, err)
		}
		if err := json.Unmarshal([]byte(from), &m.Endpoints); err != nil {
			return nil, fmt.Errorf("hook mail %d's addresses: %w", m.ID, err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Recipients is everybody to mail of an event that came through an address, each once however
// many of their entries choose it.
func (s *Store) Recipients(ctx context.Context, event string, endpoint int64) ([]string, error) {
	mails, err := s.HookMails(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range mails {
		if m.Mails(event) && m.From(endpoint) && !slices.Contains(out, m.To) {
			out = append(out, m.To)
		}
	}
	return out, nil
}

// AddHookMail adds somebody to mail.
func (s *Store) AddHookMail(ctx context.Context, m HookMail) (*HookMail, error) {
	m, events, from, err := s.checkHookMail(ctx, m)
	if err != nil {
		return nil, err
	}
	res, err := s.writer.ExecContext(ctx,
		`INSERT INTO hook_mails (to_address, events, endpoints, created_at) VALUES (?, ?, ?, ?)`,
		m.To, events, from, unix(s.Now()))
	if err != nil {
		return nil, fmt.Errorf("add a hook mail: %w", err)
	}
	if m.ID, err = res.LastInsertId(); err != nil {
		return nil, err
	}
	s.changed()
	return &m, nil
}

// SetHookMail changes whom an entry mails, and of what.
func (s *Store) SetHookMail(ctx context.Context, m HookMail) (*HookMail, error) {
	m, events, from, err := s.checkHookMail(ctx, m)
	if err != nil {
		return nil, err
	}
	// Saving what is there already is not a change for a backup to carry.
	var to, was, wasFrom string
	err = s.reader.QueryRowContext(ctx, `SELECT to_address, events, endpoints FROM hook_mails WHERE id = ?`, m.ID).
		Scan(&to, &was, &wasFrom)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, NotFound("There is nobody like that mailed.")
	}
	if err != nil {
		return nil, fmt.Errorf("read a hook mail: %w", err)
	}
	if to == m.To && was == events && wasFrom == from {
		return &m, nil
	}
	res, err := s.writer.ExecContext(ctx,
		`UPDATE hook_mails SET to_address = ?, events = ?, endpoints = ? WHERE id = ?`, m.To, events, from, m.ID)
	if err != nil {
		return nil, fmt.Errorf("set a hook mail: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, NotFound("There is nobody like that mailed.")
	}
	s.changed()
	return &m, nil
}

// RemoveHookMail stops mailing somebody.
func (s *Store) RemoveHookMail(ctx context.Context, id int64) error {
	res, err := s.writer.ExecContext(ctx, `DELETE FROM hook_mails WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("remove a hook mail: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return NotFound("There is nobody like that mailed.")
	}
	s.changed()
	return nil
}

// checkHookMail refuses an entry that would mail nobody or nothing, or from an address there is
// not, and says its events and addresses as kept.
func (s *Store) checkHookMail(ctx context.Context, m HookMail) (HookMail, string, string, error) {
	m.To = strings.TrimSpace(m.To)
	switch {
	case m.To == "":
		return m, "", "", Invalid("Say whom to mail.")
	case !strings.Contains(m.To, "@"):
		return m, "", "", Invalid("%q is not an email address.", m.To)
	case len(m.Events) == 0:
		return m, "", "", Invalid("Choose at least one kind of event to mail.")
	}
	m.Endpoints = slices.Compact(slices.Sorted(slices.Values(m.Endpoints)))
	if m.Endpoints == nil {
		m.Endpoints = []int64{}
	}
	for _, id := range m.Endpoints {
		if _, err := s.HookEndpoint(ctx, id); err != nil {
			if errors.Is(err, ErrNotFound) {
				return m, "", "", Invalid("There is no address %d to mail from.", id)
			}
			return m, "", "", err
		}
	}
	events, err := json.Marshal(m.Events)
	if err != nil {
		return m, "", "", err
	}
	from, err := json.Marshal(m.Endpoints)
	return m, string(events), string(from), err
}
