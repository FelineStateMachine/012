package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMachineIDIsKept(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	t.Setenv("O12_CONFIG_DIR", dir)
	id := machineID()
	if len(id) != 32 {
		t.Fatalf("id %q", id)
	}
	if again := machineID(); again != id {
		t.Errorf("second id %q, first %q", again, id)
	}
	if st, err := os.Stat(filepath.Join(dir, "machine-id")); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("file %v %v", st, err)
	}
}
