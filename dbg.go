// +build debug

package wavelet

import (
	"fmt"
	"sync"
	"time"
)

var debugMutex sync.Mutex
var lastDebugLog time.Time

func dbg(args ...interface{}) {
	debugMutex.Lock()
	defer debugMutex.Unlock()

	if time.Since(lastDebugLog).Milliseconds() > 100 {
		fmt.Println(args...)

		lastDebugLog = time.Now()
	}
}
