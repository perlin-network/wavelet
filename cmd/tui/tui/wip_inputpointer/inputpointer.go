package inputpointer

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
)

var ErrUnknownPtrType = errors.New("Unknown ptr type, only supports int, float or string")

type Input struct {
	tview.InputField
	underlying reflect.Value
}

func New(ptr interface{}) {
	i := &Input{
		InputField: *(tview.NewInputField()),
		underlying: reflect.Indirect(reflect.ValueOf(ptr)),
	}

	i.SetText(fmt.Sprint(i.underlying.Interface()))

	i.Set
}

func (i *Input) Set(input string) error {
	switch i.underlying.Kind() {
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int8:
		v, err := strconv.ParseInt(input, 10, 64)
		if err != nil {
			return err
		}

		i.underlying.SetInt(v)

	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(input, 64)
		if err != nil {
			return err
		}

		i.underlying.SetFloat(v)

	case reflect.String:
		i.underlying.SetString(input)

	default:
		return ErrUnknownPtrType
	}

	return nil
}

type String struct {
	tview.InputField
	Changed bool
	ptr     *string
}

func NewString(str *string) *String {
	i := tview.NewInputField()
	i.SetText(*str)

	return &String{
		InputField: *i,
		ptr:        str,
	}
}

func (s *String) InputHandler() func(*tcell.EventKey, func(p tview.Primitive)) {
	return func(key *tcell.EventKey, setFocus func(p tview.Primitive)) {
		s.Changed = true
		s.InputField.InputHandler()(key, setFocus)
	}
}

func (s *String) Draw(screen tcell.Screen) {
	if !s.Changed {
		s.InputField.SetText(*s.ptr)
	}

	s.InputField.Draw(screen)
}
