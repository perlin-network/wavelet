package forms

import (
	"errors"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/center"
	"github.com/perlin-network/wavelet/cmd/tui/tui/errdialog"
	"github.com/perlin-network/wavelet/cmd/tui/tui/inputcomplete"
)

var (
	// ErrNotStruct is returned if the interface given is not a struct
	ErrNotStruct = errors.New("interface given is not a struct")

	DefaultWidth  = 80
	DefaultHeight = 100
)

type Form struct {
	*tview.Form

	// default: 50
	FieldWidth int

	// default: DefaultWidth
	Width int

	// default: DefaultHeight
	Height int

	pairs []Pair

	submitted chan struct{}
}

// New creates a new form
func New() *Form {
	form := tview.NewForm()
	submitted := make(chan struct{})

	form.SetCancelFunc(func() {
		close(submitted)
	})

	form.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyCtrlC {
			close(submitted)
			return nil
		}

		return ev
	})

	return &Form{
		Form:       form,
		FieldWidth: 50,
		Width:      DefaultWidth,
		Height:     DefaultHeight,
		submitted:  submitted,
	}
}

// Add adds pairs
func (f *Form) Add(pairs ...Pair) {
	f.pairs = append(f.pairs, pairs...)

	for _, pair := range pairs {
		field := inputcomplete.New()
		field.SetLabel(pair.Name)
		field.SetText(pair.Default)
		field.SetFieldWidth(f.FieldWidth)
		field.SetAcceptanceFunc(pair.Validator)
		field.SetFieldWidth(f.Width)

		if pair.Completer != nil {
			field.Completer = pair.Completer
			field.Complete.SetBackgroundColor(tcell.ColorWhite)
			field.Complete.SetSelectedBackgroundColor(tcell.ColorGrey)
			field.Complete.SetMainTextColor(tcell.ColorGrey)
			field.Complete.SetSelectedTextColor(tcell.ColorWhite)
		}
	}
}

// Spawn blocks until the user submits or cancels the input. If the user
// submitted, true is returned. If the user cancelled, false.
func (f *Form) Spawn() bool {
	// Save the old primitive
	oldPrimitive := tview.GetRoot()

	if oldPrimitive != nil {
		defer tview.SetRoot(oldPrimitive, true)
		defer tview.SetFocus(oldPrimitive)
	}

	c := center.New(f)
	c.MaxHeight = f.Height
	c.MaxWidth = f.Width

	var lastError error

	f.AddButton("Submit", func() {
		if err := f.done(); err != nil {
			lastError = err
		}

		// Run the submit loop
		f.submitted <- struct{}{}
	})

	f.AddButton("Cancel", func() {
		close(f.submitted)
	})

	for {
		tview.SetRoot(c, true)
		tview.SetFocus(c)

		tview.Draw()

		// Loop for each submit
		_, ok := <-f.submitted

		// If the user cancelled the prompt
		if !ok {
			return false
		}

		// If nothing is wrong with the user data
		if lastError == nil {
			return true
		}

		// Primitive at this point is still the Form primitive.

		// Call the dialog and block until user hits Ok.
		errdialog.CallDialog(lastError.Error(), nil)

		// Reset the last error to nil
		lastError = nil

		// Loop again, wait for the right response.
	}
}

// validate all forms
func (f *Form) done() error {
	for _, pair := range f.pairs {
		item := f.GetFormItemByLabel(pair.Name)
		ip, ok := item.(*tview.InputField)
		if !ok {
			// shouldn't ever happen
			panic(pair.Name + " is not an input field!")
		}

		if err := pair.Value(ip.GetText()); err != nil {
			return err
		}
	}

	return nil
}

/*
// Form embeds tview's Form
type Form struct {
	*tview.Form

	ptr    interface{}
	values map[string]*reflect.Value
}

func Call(i interface{}) (f *Form, err error) {
	f = &Form{
		Form:   tview.NewForm(),
		values: map[string]*reflect.Value{},
	}

	elem := reflect.ValueOf(i).Elem()
	if elem.Kind() != reflect.Struct {
		return nil, ErrNotStruct
	}

	for i := 0; i < elem.NumField(); i++ {
		val := elem.Field(i)

		// If the field is not something we could set
		if !val.CanSet() {
			continue
		}

		field := elem.Type().Field(i)

		name := field.Tag.Get("form")
		if name == "" {
			continue
		}

		// Set the field into a map for later use
		f.values[name] = &val

		switch val.Kind() {
		case reflect.Int, reflect.Uint,
			reflect.Int8, reflect.Uint8,
			reflect.Int16, reflect.Uint16,
			reflect.Int32, reflect.Uint32,
			reflect.Int64, reflect.Uint64:

			f.AddInputField(name, "", FieldWidth, intChecker, nil)

		default:
			f.AddInputField(name, "", FieldWidth, nil, nil)
		}
	}

	var formError error

	f.AddButton("Submit", func() {
		for name, value := range f.values {
			formItem := f.GetFormItemByLabel(name)
			field, ok := formItem.(*tview.InputField)
			if !ok {
				// Shouldn't ever happen
				panic("Field " + name + " is not tview.InputField!")
			}

			switch value.Kind() {
			case reflect.String:
				value.SetString(field.GetText())

			case reflect.Int, reflect.Int8, reflect.Int16,
				reflect.Int32, reflect.Int64:

				i, err := strconv.ParseInt(field.GetText(), 10, 64)
				if err != nil {
					formError = err
					return
				}

				value.SetInt(i)

			case reflect.Uint, reflect.Uint8, reflect.Uint16,
				reflect.Uint32, reflect.Uint64:

				u, err := strconv.ParseUint(field.GetText(), 10, 64)
				if err != nil {
					formError = err
					return
				}

				value.SetUint(u)
			}
		}
	})

	// This for loop should loop until formError is nil
	for {
	}

	return f, nil
}
*/
