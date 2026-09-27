//go:build !darwin && !windows

package keyring

// System is the Secret Service, through secret-tool.
func System() Store { return secretService{run: run} }
