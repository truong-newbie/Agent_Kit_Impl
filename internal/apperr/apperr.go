// Package apperr defines typed errors and CLI exit codes for DTKit.
package apperr

import (
	"context"
	"errors"
	"fmt"
)

const (
	ExitOK        = 0
	ExitInternal  = 1
	ExitUsage     = 2
	ExitConflict  = 3
	ExitRuntime   = 4
	ExitCancelled = 130
)

// Error carries a machine-readable code, exit code, and optional path/hint.
type Error struct {
	Code     string `json:"code"`
	ExitCode int    `json:"-"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Err      error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Path)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func New(exitCode int, code, msg string) *Error {
	return &Error{Code: code, ExitCode: exitCode, Message: msg}
}

func Wrap(exitCode int, code, msg string, err error) *Error {
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return &Error{Code: code, ExitCode: exitCode, Message: msg, Err: err}
}

func WithPath(err *Error, path string) *Error {
	err.Path = path
	return err
}

func WithHint(err *Error, hint string) *Error {
	err.Hint = hint
	return err
}

func Usage(code, msg string) *Error    { return New(ExitUsage, code, msg) }
func Conflict(code, msg string) *Error { return New(ExitConflict, code, msg) }
func Runtime(code, msg string) *Error  { return New(ExitRuntime, code, msg) }
func Cancelled(msg string) *Error      { return New(ExitCancelled, "cancelled", msg) }

// ExitCodeOf extracts the DTKit exit code from err.
func ExitCodeOf(err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, context.Canceled) {
		return ExitCancelled
	}
	var appErr *Error
	if errors.As(err, &appErr) && appErr.ExitCode != 0 {
		return appErr.ExitCode
	}
	return ExitInternal
}

// AsError converts any error into an *Error for structured reporting.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	if errors.Is(err, context.Canceled) {
		return Cancelled("operation cancelled")
	}
	return Wrap(ExitInternal, "internal_error", err.Error(), err)
}
