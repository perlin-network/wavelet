// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package api

import (
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"sync"
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
	registry *sessionRegistry
	id       string
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

	return r.sessions[id], nil
}
