package parser

import "fmt"

// ErrorCode 用于区分不同的拒绝原因。
type ErrorCode string

const (
	ErrMalformed      ErrorCode = "malformed_input"
	ErrMissingField   ErrorCode = "missing_required_field"
	ErrTypeMismatch   ErrorCode = "type_mismatch"
	ErrEmptyEntry     ErrorCode = "empty_entry"
	ErrDuplicateEntry ErrorCode = "duplicate_entry"
	ErrUnsupported    ErrorCode = "unsupported_format"
)

// Error 是对底层解析库异常的统一封装，不向调用方原样透传底层报错。
type Error struct {
	Code    ErrorCode
	Format  string
	Entry   string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	return fmt.Sprintf("parse error: code=%s format=%s entry=%q: %s", e.Code, e.Format, e.Entry, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// newError 统一封装解析过程中的拒绝原因与底层库异常。
func newError(code ErrorCode, format, entry, message string, cause error) *Error {
	return &Error{Code: code, Format: format, Entry: entry, Message: message, Cause: cause}
}
