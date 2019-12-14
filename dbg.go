// +build debug

package wavelet

import (
	"fmt"
	"sync"
	"time"
)

var debugMutex sync.Mutex
var lastDebugLog time.Time

var debugLogsCount map[string]int = make(map[string]int)
var debugLogsTime map[string]time.Time = make(map[string]time.Time)
var debugLogsOrder []string

func dbg(args ...interface{}) {
	debugMutex.Lock()
	defer debugMutex.Unlock()

	msg := fmt.Sprintln(args...)

	if _, exists := debugLogsCount[msg]; !exists {
		debugLogsTime[msg] = time.Now()
		debugLogsOrder = append(debugLogsOrder, msg)
	}

	debugLogsCount[msg]++

	if time.Since(lastDebugLog).Milliseconds() > 100 {
		for _, msg := range debugLogsOrder {
			fmt.Printf("[x%d] %s", debugLogsCount[msg], msg)
		}

		for msg := range debugLogsCount {
			delete(debugLogsCount, msg)
			delete(debugLogsTime, msg)
		}

		debugLogsOrder = debugLogsOrder[:0]

		lastDebugLog = time.Now()
	}
}
