package journal

import "errors"

// ErrNotFound is returned when a requested transaction, price, or account
// declaration cannot be found in the journal. Callers should check with
// errors.Is rather than inspecting the error message string.
var ErrNotFound = errors.New("not found")

// ErrInvalidDate is returned when a date string does not parse as YYYY-MM-DD.
// Callers should check with errors.Is rather than inspecting the error message string.
var ErrInvalidDate = errors.New("invalid date")
