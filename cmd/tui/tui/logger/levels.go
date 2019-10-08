package logger

// Level provides a logging level interface. An example for using Error:
//
//    l.Level(logger.WithError(err).Wrap("An error has occured").
//        F("id", "%d", id))
//
type Level interface {
	Full() string
	Short() (string, string)
	F(key, format string, values ...interface{}) Level
}

// To save you from reading this file just has
// - WithError
// - WithWarning
// - WithInfo
// - WithSuccess

// Error is red
type Error struct {
	*generic

	wrap string
	err  error
}

var _ Level = (*Error)(nil)

func WithError(err error) *Error {
	return &Error{
		generic: newGeneric("red"),
		err:     err,
	}
}

// Wrap wraps the error with "wrap: error"
func (err *Error) Wrap(wrap string) *Error {
	err.wrap = wrap
	return err
}

func (err *Error) F(key, format string, values ...interface{}) Level {
	err.f(key, format, values...)
	return err
}

func (err *Error) Error() string {
	if err.wrap != "" {
		return err.wrap + ": " + err.err.Error()
	}

	return err.err.Error()
}

// Short fits into a list
func (err *Error) Short() (string, string) {
	return err.wrapShort(err.Error()), err.shortkeys()
}

// Full fits into a full textview
func (err *Error) Full() string {
	return err.wrapFull("ERROR: "+err.Error()) + "\n" + err.fullkeys()
}

// Warning is yellow
type Warning struct {
	*generic
	warn string
}

var _ Level = (*Warning)(nil)

func WithWarning(warn string) *Warning {
	return &Warning{
		generic: newGeneric("yellow"),
		warn:    warn,
	}
}

func (w *Warning) F(key, format string, values ...interface{}) Level {
	w.f(key, format, values...)
	return w
}

// Short fits into a list
func (w *Warning) Short() (string, string) {
	return w.wrapShort(w.warn), w.shortkeys()
}

// Full fits into a full textview
func (w *Warning) Full() string {
	return w.wrapFull("WARNING: "+w.warn) + "\n" + w.fullkeys()
}

// Info is blue/cyanish
type Info struct {
	*generic
	info string
}

var _ Level = (*Info)(nil)

func WithInfo(info string) *Info {
	return &Info{
		generic: newGeneric("teal"),
		info:    info,
	}
}

func (i *Info) F(key, format string, values ...interface{}) Level {
	i.f(key, format, values...)
	return i
}

// Short fits into a list
func (i *Info) Short() (string, string) {
	return i.wrapShort(i.info), i.shortkeys()
}

// Full fits into a full textview
func (i *Info) Full() string {
	return i.wrapFull("INFO: "+i.info) + "\n" + i.fullkeys()
}

// Success is green/limeish
type Success struct {
	*generic
	success string
}

var _ Level = (*Success)(nil)

func WithSuccess(success string) *Success {
	return &Success{
		generic: newGeneric("lime"),
		success: success,
	}
}

func (s *Success) F(key, format string, values ...interface{}) Level {
	s.f(key, format, values...)
	return s
}

// Short fits into a list
func (s *Success) Short() (string, string) {
	return s.wrapShort(s.success), s.shortkeys()
}

// Full fits into a full textview
func (s *Success) Full() string {
	return s.wrapFull("SUCCESS: "+s.success) + "\n" + s.fullkeys()
}
