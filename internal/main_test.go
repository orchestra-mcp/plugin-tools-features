package internal

import (
	"os"
	"testing"
)

// TestMain points HOME at a throwaway directory for the whole test binary.
// Tool handlers reach the globaldb singleton (~/.orchestra/db/global.db), and
// go test runs packages as separate processes in parallel. Without this, the
// internal and internal/tools binaries open the same real global.db at once,
// one of them hits SQLITE_BUSY while setting pragmas, and globaldb caches that
// error for every later test in the binary.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "plugin-tools-features-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
