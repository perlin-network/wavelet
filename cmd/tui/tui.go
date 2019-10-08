package main

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/cmd/tui/tui/forms"
	"github.com/perlin-network/wavelet/cmd/tui/tui/inputcomplete"
)

func srvCompletion(text string) []inputcomplete.Completion {
	completions := make([]inputcomplete.Completion, 0, len(srv.History.Store))

	for _, e := range srv.History.Store {
		if text == "" || strings.Contains(e.ID, text) {
			completions = append(completions, inputcomplete.Completion{
				Visual:  e.String(),
				Replace: e.ID,
			})
		}
	}

	return completions
}

func getRecipientFormPair(recipient [wavelet.SizeAccountID]byte) forms.Pair {
	return forms.Pair{
		Name: "Recipient",
		Value: func(output string) error {
			if len(output) == 64 {
				buf, err := hex.DecodeString(output)
				if err != nil {
					return err
				}

				copy(recipient[:], buf)
				return nil
			}

			return fmt.Errorf(
				"Invalid recipient length, expected %d, got %d",
				wavelet.SizeAccountID, len(output),
			)
		},
		Validator: forms.ORValidators(
			forms.LetterValidator(), forms.IntValidator(),
		),
		Completer: srvCompletion,
	}
}
