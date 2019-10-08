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
			l.Level(Ftorespect(logger.WithInfo("Hello, 世界"), now))
		case 1:
			l.Level(Ftorespect(logger.WithSuccess("Ayy, something worked").
				F("mood", "%s", "pretty nice"), now))
		case 2:
			l.Level(Ftorespect(logger.WithWarning("oh no something crapped out").
				F("mood", "%s", "what do"), now))
		case 3:
			l.Level(Ftorespect(logger.WithError(errors.New("crash and burn")).
				F("mood", "%s", "panic"), now))
		}
	}
}

func Ftorespect(lvl logger.Level, now time.Time) logger.Level {
	lvl.F("rand_int", "%d", rand.Int()).
		F("rand_int", "%d", rand.Int()).
		F("time", "%s", now.Format(time.RFC3339Nano)).
		F("unix", "%s", now.UnixNano()).
		F("some strings", "[red]kinda weird").
		F("smol struct", "%#v", struct{ d int }{13})

	return lvl
}
