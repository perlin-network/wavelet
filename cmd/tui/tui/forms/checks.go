package forms

import "unicode"

// Validator is the function called on each character input, to validate
// if the input is invalid.
type Validator func(text string, last rune) bool

// IntValidator validates that the input is a valid integer.
func IntValidator() Validator {
	return func(text string, last rune) bool {
		last -= '0'
		if 0 <= last && last <= 9 {
			return true
		}

		return false
	}
}

// LetterValidator validates that the input is a valid letter.
func LetterValidator() Validator {
	return func(text string, last rune) bool {
		return unicode.IsLetter(last)
	}
}

// RuneValidator creates a validator from a callback which takes in a rune and
// return a bool. This callback could be from the unicode package, such as
// unicode.IsLetter.
func RuneValidator(r func(rune) bool) Validator {
	return func(text string, last rune) bool {
		return r(last)
	}
}

// ORValidators creates a validator from multiple validators. If one of the
// validators pass, the input is valid.
func ORValidators(validators ...Validator) Validator {
	return func(text string, last rune) bool {
		for _, v := range validators {
			if v(text, last) {
				return true
			}
		}

		return false
	}
}
