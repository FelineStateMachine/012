package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMachineIDIsKept(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "012")
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
