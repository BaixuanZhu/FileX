package errs

import (
	"encoding/json"
	"fmt"
	"io"
)

const SchemaVersion = 1

type Code string

const (
	EInternal     Code = "E_INTERNAL"
	EUsage        Code = "E_USAGE"
	EDenied       Code = "E_DENIED"
	ELocked       Code = "E_LOCKED"
	ENotFound     Code = "E_NOT_FOUND"
	EConflict     Code = "E_CONFLICT"
	EEncoding     Code = "E_ENCODING"
	EIO           Code = "E_IO"
	ETooLarge     Code = "E_TOO_LARGE"
	EUnsupported  Code = "E_UNSUPPORTED"
)

func (c Code) ExitCode() int {
	switch c {
	case EInternal:
		return 1
	case EUsage:
		return 2
	case EDenied:
		return 3
	case ELocked:
		return 4
	case ENotFound:
		return 5
	case EConflict:
		return 6
	case EEncoding:
		return 7
	case EIO:
		return 8
	case ETooLarge:
		return 9
	case EUnsupported:
		return 10
	default:
		return 1
	}
}

type Error struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	Retryable bool   `json:"retryable"`

	// Structured refusal details: an agent can branch on these instead of
	// parsing a sentence, and a caller can offer the alternative it names.
	Reason           string `json:"reason,omitempty"`
	SaferAlternative string `json:"safer_alternative,omitempty"`
	GuardClass       string `json:"guard_class,omitempty"`
}

// DeniedByGuard builds the refusal returned by the circuit breaker.
func DeniedByGuard(message, class, reason, alternative string) *Error {
	e := Newf(EDenied, "%s (%s)", message, class)
	e.GuardClass = class
	e.Reason = reason
	e.SaferAlternative = alternative
	return e
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func New(code Code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

func Newf(code Code, format string, args ...any) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

func WithHint(err *Error, hint string) *Error {
	err.Hint = hint
	return err
}

func WithRetryable(err *Error, retryable bool) *Error {
	err.Retryable = retryable
	return err
}

type Envelope struct {
	SchemaVersion int     `json:"schema_version"`
	OK            bool    `json:"ok"`
	Data          any     `json:"data,omitempty"`
	Error         *Error  `json:"error,omitempty"`
}

func Success(data any) Envelope {
	return Envelope{
		SchemaVersion: SchemaVersion,
		OK:            true,
		Data:          data,
	}
}

func Failure(err *Error) Envelope {
	return Envelope{
		SchemaVersion: SchemaVersion,
		OK:            false,
		Error:         err,
	}
}

func WriteSuccess(w io.Writer, data any) error {
	env := Success(data)
	return json.NewEncoder(w).Encode(env)
}

func WriteFailure(w io.Writer, err *Error) error {
	env := Failure(err)
	return json.NewEncoder(w).Encode(env)
}
