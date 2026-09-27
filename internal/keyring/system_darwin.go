//go:build !fakekeyring

package keyring

// System is the macOS Keychain.
func System() Store { return keychain{run: run} }
