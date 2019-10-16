package main

import (
	"errors"
	"math/rand"
	"os"
	"time"

	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/logger"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	tview.Initialize()
	l := logger.NewLogger()

	tview.SetRoot(l, true)

	go func() {
		// Start the main blocking event loop.
		if err := tview.Run(); err != nil {
			panic(err)
		}

		os.Exit(0)
	}()

	// I wish for Go macros
	for {
		now := <-time.After(time.Millisecond * time.Duration(800+rand.Intn(1000)))

		switch rand.Intn(4) {
		case 0:
			l.Level(logger.Info, "Hello, 世界")
		case 1:
			l.Level(logger.Success, "Ayy, something worked",
				Ftorespect(now, logger.F("mood", "%s", "pretty nice"))...)
		case 2:
			l.Level(logger.Warning, "oh no something crapped out",
				logger.F("mood", "%s", "what do"))
		case 3:
			l.Error(errors.New("crash and burn"),
				logger.F("mood", "%s", "panic"))
		}
	}
}

func Ftorespect(now time.Time, F ...[2]string) [][2]string {
	return append(F,
		logger.F("rand_int", "%d", rand.Int()),
		logger.F("rand_int", "%d", rand.Int()),
		logger.F("time", "%s", now.Format(time.RFC3339Nano)),
		logger.F("unix", "%d", now.UnixNano()),
		logger.F("some strings", "[red]kinda weird"),
		logger.F("smol struct", "%#v", struct{ d int }{13}),
	)
}
