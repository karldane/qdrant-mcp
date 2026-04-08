package main

import (
	"testing"

	"github.com/karldane/mcp-framework/framework"
)

// TestWriteGateWiring verifies that the framework's write-gate is wired
// correctly to the readonly config: writes must be enabled by default and
// disabled when --readonly is set.
func TestWriteGateWiring(t *testing.T) {
	t.Run("framework default is write-enabled", func(t *testing.T) {
		// Since mcp-framework v0.2.0 the default is write-enabled (permissive).
		// Readonly mode is opt-in.
		s := framework.NewServer("test", "0.0.0")
		if !s.IsWriteEnabled() {
			t.Fatal("framework default should be write-enabled since v0.2.0")
		}
	})

	t.Run("Config.WriteEnabled=true enables writes", func(t *testing.T) {
		// Mirrors main.go: NewServerWithConfig with WriteEnabled: !cfg.ReadOnly()
		// when readonly=false.
		s := framework.NewServerWithConfig(&framework.Config{
			Name:         "test",
			Version:      "0.0.0",
			WriteEnabled: true,
		})
		if !s.IsWriteEnabled() {
			t.Fatal("expected writes to be enabled when Config.WriteEnabled=true")
		}
	})

	t.Run("Config.WriteEnabled=false disables writes", func(t *testing.T) {
		// Mirrors main.go: NewServerWithConfig with WriteEnabled: !cfg.ReadOnly()
		// when readonly=true.
		s := framework.NewServerWithConfig(&framework.Config{
			Name:         "test",
			Version:      "0.0.0",
			WriteEnabled: false,
		})
		if s.IsWriteEnabled() {
			t.Fatal("expected writes to be disabled when Config.WriteEnabled=false")
		}
	})
}
