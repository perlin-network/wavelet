package inputcomplete

type Completer func(text string) []Completion

type Completion struct {
	Visual  string // what is displayed, optional
	Replace string // what is replaced
}
