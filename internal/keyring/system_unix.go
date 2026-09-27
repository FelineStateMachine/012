//go:build !darwin && !windows && !fakekeyring

package keyring

// System is the Secret Service, through secret-tool.
func System() Store { return secretService{run: run} }
