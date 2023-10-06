// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

//go:build freebsd || netbsd
// +build freebsd netbsd

package duplicacy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"syscall"
	"unsafe"

	"github.com/pkg/xattr"
)

const (
	bsd_UF_NODUMP   = 0x1
	bsd_SF_SETTABLE = 0xffff0000
	bsd_UF_SETTABLE = 0x0000ffff

	bsdFileFlagsKey = "\x00bf"
)

var bsdIsSuperUser bool

func init() {
	bsdIsSuperUser = syscall.Geteuid() == 0
}

func (entry *Entry) ReadAttributes(fullPath string, fi os.FileInfo) error {
	if entry.IsSpecial() {
		return nil
	}

	attributes, err := xattr.LList(fullPath)
	if err != nil {
		return err
	}

	if len(attributes) > 0 {
		entry.Attributes = &map[string][]byte{}
	}
	var allErrors error
	for _, name := range attributes {
		value, err := xattr.LGet(fullPath, name)
		if err != nil {
			allErrors = errors.Join(allErrors, err)
		} else {
			(*entry.Attributes)[name] = value
		}
	}

	return allErrors
}

func (entry *Entry) ReadFileFlags(fullPath string, fileInfo os.FileInfo) error {
	stat := fileInfo.Sys().(*syscall.Stat_t)
	if stat.Flags != 0 {
		if entry.Attributes == nil {
			entry.Attributes = &map[string][]byte{}
		}
		v := make([]byte, 4)
		binary.LittleEndian.PutUint32(v, stat.Flags)
		(*entry.Attributes)[bsdFileFlagsKey] = v
	}
	return nil
}

func (entry *Entry) SetAttributesToFile(fullPath string, normalize bool) error {
	if entry.Attributes == nil || len(*entry.Attributes) == 0 || entry.IsSpecial() {
		return nil
	}
	attributes := *entry.Attributes

	if _, haveFlags := attributes[bsdFileFlagsKey]; haveFlags && len(attributes) <= 1 {
		return nil
	}

	names, err := xattr.LList(fullPath)
	if err != nil {
		return err
	}
	for _, name := range names {
		newAttribute, found := attributes[name]
		if found {
			oldAttribute, _ := xattr.LGet(fullPath, name)
			if !bytes.Equal(oldAttribute, newAttribute) {
				err = errors.Join(err, xattr.LSet(fullPath, name, newAttribute))
			}
			delete(attributes, name)
		} else {
			err = errors.Join(err, xattr.LRemove(fullPath, name))
		}
	}

	for name, attribute := range attributes {
		if len(name) > 0 && name[0] == '\x00' {
			continue
		}
		err = errors.Join(err, xattr.LSet(fullPath, name, attribute))
	}
	return err
}

func (entry *Entry) RestoreEarlyDirFlags(fullPath string, mask uint32) error {
	return nil
}

func (entry *Entry) RestoreEarlyFileFlags(f *os.File, mask uint32) error {
	return nil
}

func (entry *Entry) RestoreLateFileFlags(fullPath string, fileInfo os.FileInfo, mask uint32) error {
	if mask == 0xffffffff {
		return nil
	}

	if bsdIsSuperUser {
		mask |= ^uint32(bsd_UF_SETTABLE | bsd_SF_SETTABLE)
	} else {
		mask |= ^uint32(bsd_UF_SETTABLE)
	}

	var flags uint32

	if entry.Attributes != nil {
		if v, have := (*entry.Attributes)[bsdFileFlagsKey]; have {
			flags = binary.LittleEndian.Uint32(v)
		}
	}

	stat := fileInfo.Sys().(*syscall.Stat_t)

	flags = (flags & ^mask) | (stat.Flags & mask)

	if flags != stat.Flags {
		pPath, _ := syscall.BytePtrFromString(fullPath)
		if _, _, errno := syscall.Syscall(syscall.SYS_LCHFLAGS,
			uintptr(unsafe.Pointer(pPath)),
			uintptr(flags), 0); errno != 0 {
			return os.NewSyscallError("lchflags", errno)
		}
	}
	return nil
}
