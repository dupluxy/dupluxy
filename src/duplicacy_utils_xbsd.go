// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

//go:build freebsd || netbsd
// +build freebsd netbsd

package duplicacy

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"

	"github.com/pkg/xattr"
)

func (entry *Entry) restoreLateFileFlags(path string) error {
	if entry.Attributes == nil {
		return nil
	}
	if v, have := (*entry.Attributes)[bsdFileFlagsKey]; have {
		if _, _, errno := syscall.Syscall(syscall.SYS_LCHFLAGS, uintptr(unsafe.Pointer(syscall.StringBytePtr(path))), uintptr(v), 0); errno != 0 {
			return os.NewSyscallError("lchflags", errno)
		}
	}
	return nil
}
