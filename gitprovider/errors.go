package gitprovider

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrorCode is the closed set of failure classes core understands. Providers
// map every platform failure onto one of them so retry and back-off behave the
// same everywhere.
type ErrorCode string

const (
	CodeNotFound         ErrorCode = "not_found"
	CodeForbidden        ErrorCode = "forbidden"         // the identity lacks access
	CodeInvalidSignature ErrorCode = "invalid_signature" // webhook failed verification
	CodeConflict         ErrorCode = "conflict"          // e.g. branch exists, head moved
	CodeRateLimited      ErrorCode = "rate_limited"
	CodeValidation       ErrorCode = "validation" // bad request from core or the platform
	CodeUnsupported      ErrorCode = "unsupported"
	CodeTransient        ErrorCode = "transient" // retryable (5xx, timeouts)
	CodeInternal         ErrorCode = "internal"
)

// AllErrorCodes lists every defined code.
var AllErrorCodes = []ErrorCode{
	CodeNotFound, CodeForbidden, CodeInvalidSignature, CodeConflict,
	CodeRateLimited, CodeValidation, CodeUnsupported, CodeTransient, CodeInternal,
}

// Error is a provider failure. Message must never contain credentials, webhook
// secrets or raw payloads.
type Error struct {
	Code              ErrorCode `json:"code"`
	Message           string    `json:"message"`
	RetryAfterSeconds int       `json:"retry_after_seconds,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("gitprovider: %s: %s", e.Code, e.Message) }

// Retryable reports whether the same call may succeed later.
func (e *Error) Retryable() bool { return e.Code == CodeRateLimited || e.Code == CodeTransient }

// Is makes errors.Is(err, ErrUnsupported) match any *Error with that code.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// Sentinels for errors.Is.
var (
	ErrNotFound         = &Error{Code: CodeNotFound}
	ErrForbidden        = &Error{Code: CodeForbidden}
	ErrInvalidSignature = &Error{Code: CodeInvalidSignature}
	ErrConflict         = &Error{Code: CodeConflict}
	ErrRateLimited      = &Error{Code: CodeRateLimited}
	ErrValidation       = &Error{Code: CodeValidation}
	ErrUnsupported      = &Error{Code: CodeUnsupported, Message: "operation not supported by this provider"}
	ErrTransient        = &Error{Code: CodeTransient}
)

// Errorf builds an *Error with a formatted message.
func Errorf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// RateLimited builds a rate-limit error with a retry hint.
func RateLimited(retryAfterSeconds int, msg string) *Error {
	return &Error{Code: CodeRateLimited, Message: msg, RetryAfterSeconds: retryAfterSeconds}
}

// AsError converts any error to an *Error, wrapping unknown errors as
// [CodeInternal] with a generic message so internals never leak.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: CodeInternal, Message: "internal provider error"}
}

// HTTPStatus maps a code to the status the serve adapter returns.
func (c ErrorCode) HTTPStatus() int {
	switch c {
	case CodeNotFound:
		return 404
	case CodeForbidden:
		return 403
	case CodeInvalidSignature:
		return 401
	case CodeConflict:
		return 409
	case CodeRateLimited:
		return 429
	case CodeValidation:
		return 400
	case CodeUnsupported:
		return 501
	case CodeTransient:
		return 503
	default:
		return 500
	}
}

// Envelope is the JSON every operation answers with.
type Envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}
