package forms

import (
	"strings"

	"github.com/perlin-network/wavelet/cmd/tui/tui/inputcomplete"
)

// CompleterContains creates a Completer from a list of possible completion
// strings. `caseSens' changes if the matching should be case sensitive.
func CompleterContains(possible []string, caseSens bool) inputcomplete.Completer {
	var contains func(s, substr string) bool
	if caseSens {
		contains = strings.Contains
	} else {
		contains = func(s, substr string) bool {
			return strings.Contains(
				strings.ToLower(s),
				strings.ToLower(substr),
			)
		}
	}

	var matches = make([]inputcomplete.Completion, 0, len(possible))

	return func(text string) []inputcomplete.Completion {
		matches = matches[:0]

		for _, p := range possible {
			if contains(text, p) {
				matches = append(matches, inputcomplete.Completion{
					Replace: p,
				})
			}
		}

		return matches
	}
}
