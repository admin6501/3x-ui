package service

import (
	"errors"
	"fmt"
	"testing"
)

// TestImportedButXrayFailedIsNotAnImportFailure pins the distinction the restore
// flow depends on. The Xray restart happens after the uploaded database is
// already in place, so that failure must stay recognisable as "restored, with a
// warning" rather than collapsing into a generic error — reporting it as a
// failure is what used to leave a restored panel showing its pre-restore data
// until the admin restored a second time.
func TestImportedButXrayFailedIsNotAnImportFailure(t *testing.T) {
	cause := errors.New("failed to write configuration file")
	err := error(&ImportedButXrayFailedError{Cause: cause})

	var partial *ImportedButXrayFailedError
	if !errors.As(err, &partial) {
		t.Fatal("callers must be able to recognise the partial-success error")
	}
	if partial.Cause != cause {
		t.Errorf("Cause = %v, want the original restart error", partial.Cause)
	}
	if !errors.Is(err, cause) {
		t.Error("the underlying restart error must stay unwrappable")
	}
	if got := err.Error(); got == "" {
		t.Error("the error must describe itself")
	}

	// It survives one more layer of wrapping, which is how it travels up
	// through the controller.
	wrapped := fmt.Errorf("importDB: %w", err)
	if !errors.As(wrapped, &partial) {
		t.Fatal("wrapping must not hide the partial-success error")
	}

	// An ordinary failure must not be mistaken for it: those really do mean
	// the database was left untouched.
	if errors.As(errors.New("invalid db file format"), &partial) {
		t.Error("a plain error must not classify as partial success")
	}
}
