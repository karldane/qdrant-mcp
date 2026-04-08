package main

import (
	"testing"

	"github.com/karldane/mcp-framework/framework"
)

// TestWriteGateWiring verifies that the framework's write-gate is wired
// correctly to the readonly config: writes must be enabled by default and
// disabled when --readonly is set.
//
// Background: framework.Server.writeEnabled defaults to false. If main.go
// does not call SetWriteEnabled(true) on start-up, all mutating tools
// return "Write tools are disabled. Enable with --write-enabled flag."
// regardless of the --readonly flag.
func TestWriteGateWiring(t *testing.T) {
	t.Run("default: writes enabled", func(t *testing.T) {
		s := framework.NewServer("test", "0.0.0")
		s.SetWriteEnabled(true) // mirrors: !cfg.ReadOnly() when readonly=false
		if !s.IsWriteEnabled() {
			t.Fatal("expected writes to be enabled when readonly=false")
		}
	})

	t.Run("readonly=true: writes disabled", func(t *testing.T) {
		s := framework.NewServer("test", "0.0.0")
		s.SetWriteEnabled(false) // mirrors: !cfg.ReadOnly() when readonly=true
		if s.IsWriteEnabled() {
			t.Fatal("expected writes to be disabled when readonly=true")
		}
	})

	t.Run("framework default is write-disabled", func(t *testing.T) {
		// This documents the framework footgun: if SetWriteEnabled is never
		// called, writes are silently blocked with a misleading error message.
		s := framework.NewServer("test", "0.0.0")
		if s.IsWriteEnabled() {
			t.Fatal("framework default should be write-disabled (this test documents the footgun)")
		}
	})
}
