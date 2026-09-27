package keyring

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// System is the Windows Credential Manager, as a generic credential
// named "012:jev-api-key", through advapi32's CredReadW, CredWriteW and
// CredDeleteW.
func System() Store { return credManager{} }

var (
	advapi32   = windows.NewLazySystemDLL("advapi32.dll")
	credRead   = advapi32.NewProc("CredReadW")
	credWrite  = advapi32.NewProc("CredWriteW")
	credDelete = advapi32.NewProc("CredDeleteW")
	credFree   = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	target                  = Service + ":" + Account
)

// credential is CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type credManager struct{}

func (credManager) Name() string { return "Windows Credential Manager" }

func (credManager) Get(context.Context) (string, error) {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return "", err
	}
	var c *credential
	r, _, callErr := credRead.Call(uintptr(unsafe.Pointer(name)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		return "", wrap(callErr)
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	return string(unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)), nil
}

func (credManager) Set(_ context.Context, secret string) error {
	if len(secret) == 0 {
		return errors.New("empty key")
	}
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	user, _ := windows.UTF16PtrFromString(Account)
	comment, _ := windows.UTF16PtrFromString(Label)
	blob := []byte(secret)
	c := credential{
		Type: credTypeGeneric, TargetName: name, Comment: comment, UserName: user,
		CredentialBlobSize: uint32(len(blob)), CredentialBlob: &blob[0], Persist: credPersistLocalMachine,
	}
	if r, _, callErr := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return wrap(callErr)
	}
	return nil
}

func (credManager) Delete(context.Context) error {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	if r, _, callErr := credDelete.Call(uintptr(unsafe.Pointer(name)), credTypeGeneric, 0); r == 0 {
		return wrap(callErr)
	}
	return nil
}

func wrap(err error) error {
	if errors.Is(err, windows.ERROR_NOT_FOUND) {
		return ErrNotFound
	}
	return fmt.Errorf("credential manager: %w", err)
}
