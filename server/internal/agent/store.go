package agent

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Store is a thread-safe in-memory registry of tool-call confirmations that are
// awaiting user input. Create one instance at startup and share it across
// handlers.
type Store struct {
	mu      sync.Mutex
	pending map[pgtype.UUID]*PendingConfirm
}

// NewStore creates an empty Store ready for use.
func NewStore() *Store {
	return &Store{pending: make(map[pgtype.UUID]*PendingConfirm)}
}

// Create registers a new pending confirmation for the given session and tool
// call. The caller must eventually call Delete (on timeout) or rely on Confirm
// to remove the entry.
func (s *Store) Create(sessionID pgtype.UUID, toolCall ToolCallDecision, timeout time.Duration) *PendingConfirm {
	pc := &PendingConfirm{
		ID:        newUUID(),
		SessionID: sessionID,
		ToolCall:  toolCall,
		ResultCh:  make(chan bool), // unbuffered: Store.Confirm uses a non-blocking send
		ExpiresAt: time.Now().Add(timeout),
	}
	s.mu.Lock()
	s.pending[pc.ID] = pc
	s.mu.Unlock()
	return pc
}

// Confirm delivers the user's decision to the waiting agent goroutine.
//
// sessionID is verified against the stored value so one user cannot approve
// another user's tool call. ErrConfirmExpired is returned when the confirmation
// is not found (timed out or unknown) or when the agent goroutine has already
// moved on past its select statement.
func (s *Store) Confirm(confirmID, sessionID pgtype.UUID, approved bool) error {
	s.mu.Lock()
	pc, ok := s.pending[confirmID]
	if ok {
		if pc.SessionID != sessionID {
			s.mu.Unlock()
			// Return not-found to avoid leaking that a different session's confirm exists.
			return ErrConfirmExpired
		}
		delete(s.pending, confirmID)
	}
	s.mu.Unlock()

	if !ok {
		return ErrConfirmExpired
	}

	// Non-blocking send: if the agent goroutine already exited its select (due to
	// timeout or context cancellation), no goroutine is reading from ResultCh and
	// the send falls through to default, giving the caller a meaningful error.
	select {
	case pc.ResultCh <- approved:
		return nil
	default:
		return ErrConfirmExpired
	}
}

// Delete removes a pending confirmation without delivering a result. The agent
// goroutine calls this when its confirmation timer fires.
func (s *Store) Delete(confirmID pgtype.UUID) {
	s.mu.Lock()
	delete(s.pending, confirmID)
	s.mu.Unlock()
}

// newUUID generates a random RFC 4122 version-4 UUID.
func newUUID() pgtype.UUID {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return pgtype.UUID{Bytes: b, Valid: true}
}

// UUIDString formats a pgtype.UUID as a canonical hyphenated hex string.
func UUIDString(u pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}
