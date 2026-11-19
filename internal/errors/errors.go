// Package errors defines typed sentinel errors used across the application.
// Wrapping with fmt.Errorf("...: %w", pkgerrors.ErrXxx) keeps errors.Is working
// at package boundaries (SPEC rule 11).
package errors

import "errors"

// Application error categories. Network/timeout/cancellation errors are
// surfaced distinctly so the scheduler and diagnostic engine can react
// appropriately (SPEC §4.4, §9.2).
var (
	ErrConfig       = errors.New("config error")
	ErrStorage      = errors.New("storage error")
	ErrMigration    = errors.New("migration error")
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrTimeout      = errors.New("timeout")
	ErrCancelled    = errors.New("cancelled")
	ErrUnsupported  = errors.New("unsupported")
)
