package cmdutil

import "fmt"

// rewrittenError prints msg and unwraps to cause.
type rewrittenError struct {
	msg   string
	cause error
}

func (e *rewrittenError) Error() string { return e.msg }
func (e *rewrittenError) Unwrap() error { return e.cause }

// Rewritef returns an error whose message is the formatted text, without the
// text of cause. errors.Is and errors.As still match cause.
func Rewritef(cause error, format string, args ...any) error {
	return &rewrittenError{msg: fmt.Sprintf(format, args...), cause: cause}
}
