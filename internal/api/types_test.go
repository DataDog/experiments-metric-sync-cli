package api

import "testing"

func TestSkippedIsTerminalButNotFailure(t *testing.T) {
	if !IsTerminalStatus("skipped") {
		t.Fatal("skipped should be terminal")
	}
	if IsFailureStatus("skipped") {
		t.Fatal("skipped should be treated as no-op success, not failure")
	}
}

func TestFailedIsFailure(t *testing.T) {
	if !IsTerminalStatus("failed") {
		t.Fatal("failed should be terminal")
	}
	if !IsFailureStatus("failed") {
		t.Fatal("failed should be treated as failure")
	}
}
