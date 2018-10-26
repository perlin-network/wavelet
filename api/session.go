package api

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
)

// registry represents a thread-safe session registry.
type registry struct {
	sync.Mutex
	Sessions map[string]*session
}

// session represents a single user session.
type session struct {
	registry    *registry
	renewTime   *time.Time // should be used atomically
	ID          string
	Permissions ClientPermissions
}

// newSessionRegistry creates a new sessions registry.
func newSessionRegistry() *registry {
	return &registry{
		Sessions: make(map[string]*session),
	}
}

// getSession returns session by id.
func (r *registry) getSession(id string) (*session, bool) {
	r.Lock()
	sess, ok := r.Sessions[id]
	r.Unlock()
	return sess, ok
}

// newSession creates a new session and stores it in registry.
func (r *registry) newSession(permissions ClientPermissions) (*session, error) {
	r.Lock()
	numSessions := len(r.Sessions)
	r.Unlock()

	if numSessions > MaxAllowableSessions {
		return nil, errors.New("too many sessions active")
	}

	currentTime := time.Now()
	id := mustUUID(uuid.NewV4())
	sess := &session{
		registry:    r,
		renewTime:   &currentTime,
		ID:          id,
		Permissions: permissions,
	}

	r.Lock()
	r.Sessions[id] = sess
	r.Unlock()

	return sess, nil
}

// Recycle will remove stale sessions.
func (r *registry) Recycle() {
	r.Lock()
	defer r.Unlock()

	sessionTimeout := MaxSessionTimeoutMinues * time.Minute
	currentTime := time.Now()

	for k, sess := range r.Sessions {
		t := *sess.loadRenewTime()
		if currentTime.Sub(t) > sessionTimeout {
			delete(r.Sessions, k)
		}
	}
}

// renew updates a life time.
func (s *session) renew() {
	t := time.Now()
	s.storeRenewTime(&t)
}

func (s *session) loadRenewTime() *time.Time {
	return (*time.Time)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.renewTime))))
}

func (s *session) storeRenewTime(t *time.Time) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.renewTime)), unsafe.Pointer(t))
}

func mustUUID(id uuid.UUID, _ error) string {
	return id.String()
}
