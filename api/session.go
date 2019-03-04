package api

import (
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"sync"
	"sync/atomic"
	"time"
)

const (
	KeyToken   = "token"
	KeySession = "session"

	HeaderSessionToken   = "X-Session-Token"
	MaxAllowableSessions = 50000
)

// sessionRegistry represents a thread-safe session registry.
type sessionRegistry struct {
	sync.Mutex
	sessions map[string]*session
}

// session represents a single user session.
type session struct {
	registry  *sessionRegistry
	renewTime atomic.Value

	id string
}

// newSessionRegistry creates a new sessions registry.
func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{
		sessions: make(map[string]*session),
	}
}

// getSession returns session by id.
func (r *sessionRegistry) getSession(id string) (*session, bool) {
	r.Lock()
	defer r.Unlock()

	session, available := r.sessions[id]
	return session, available
}

// newSession creates a new session and stores it in registry.
func (r *sessionRegistry) newSession() (*session, error) {
	r.Lock()
	defer r.Unlock()

	if len(r.sessions) >= MaxAllowableSessions {
		return nil, errors.New("too many sessions active")
	}

	id := uuid.New().String()

	r.sessions[id] = &session{
		registry: r,
		id:       id,
	}

	r.sessions[id].storeRenewTime(time.Now())

	return r.sessions[id], nil
}

// renew updates a life time.
func (s *session) renew() {
	s.storeRenewTime(time.Now())
}

func (s *session) loadRenewTime() time.Time {
	return s.renewTime.Load().(time.Time)
}

func (s *session) storeRenewTime(t time.Time) {
	s.renewTime.Store(t)
}
