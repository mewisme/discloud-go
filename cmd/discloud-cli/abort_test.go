package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/ncruces/zenity"
)

func TestIsUserAbort(t *testing.T) {
	t.Parallel()
	cases := []error{
		huh.ErrUserAborted,
		context.Canceled,
		tea.ErrInterrupted,
		zenity.ErrCanceled,
		fmt.Errorf("file: %w", context.Canceled),
		errors.New("aborted"),
	}
	for _, err := range cases {
		if !isUserAbort(err) {
			t.Fatalf("expected abort: %v", err)
		}
	}
	if isUserAbort(errors.New("boom")) {
		t.Fatal("real error must not be abort")
	}
}
