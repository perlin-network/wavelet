package logger

import "sync"

type stringmap struct {
	sync.Mutex
	store [][2]string
}

func (sm *stringmap) get() [][2]string {
	sm.Lock()
	defer sm.Unlock()

	return sm.store
}

func (sm *stringmap) in(key string) (string, bool) {
	sm.Lock()
	defer sm.Unlock()

	for _, s := range sm.store {
		if s[0] == key {
			return s[1], true
		}
	}

	return "", false
}

func (sm *stringmap) set(key, value string) {
	sm.Lock()
	defer sm.Unlock()

	for i, s := range sm.store {
		if s[0] == key {
			sm.store[i][1] = value
			return
		}
	}

	sm.store = append(sm.store, [2]string{key, value})
}
