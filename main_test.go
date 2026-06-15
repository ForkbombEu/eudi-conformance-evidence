package main

import (
	"testing"
)

func TestRunNoArgs(t *testing.T) {
	err := run(nil)
	if err == nil {
		t.Error("expected error for no args")
	}
}

func TestRunVersion(t *testing.T) {
	err := run([]string{"version"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunVersionDoubleDash(t *testing.T) {
	err := run([]string{"--version"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := run([]string{"nonexistent"})
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestRunWebRejectsInvalidFlag(t *testing.T) {
	err := run([]string{"web", "--not-a-real-flag"})
	if err == nil {
		t.Error("expected error for invalid web flag")
	}
}
