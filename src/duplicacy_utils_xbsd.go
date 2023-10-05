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
	"unsafe"

	"github.com/pkg/xattr"
)

const bsd_UF_NODUMP = 0x1

const bsdFileFlagsKey = "\x00bf"

func excludedByAttribute(attributes map[string][]byte) bool {
	_, excluded := attributes["duplicacy_exclude"]
	if !excluded {
		value, ok := attributes[bsdFileFlagsKey]
		excluded = ok && (binary.LittleEndian.Uint32(value) & bsd_UF_NODUMP) != 0
	}
	return excluded
}

func (entry *Entry) ReadAttributes(top string) {
	fullPath := filepath.Join(top, entry.Path)
	fileInfo, err := os.Lstat(fullPath)
	if err != nil {
		return
	}

	if !entry.IsSpecial() {
		attributes, _ := xattr.LList(fullPath)

		if len(attributes) > 0 {
			entry.Attributes = &map[string][]byte{}
			for _, name := range attributes {
				attribute, err := xattr.LGet(fullPath, name)
				if err == nil {
					(*entry.Attributes)[name] = attribute
				}
			}
		}
	}
	if err := entry.readFileFlags(fileInfo); err != nil {
		LOG_INFO("ATTR_BACKUP", "Could not backup flags for file %s: %v", fullPath, err)
	}
}

func (entry *Entry) SetAttributesToFile(fullPath string) {
	if !entry.IsSpecial() {
		names, _ := xattr.LList(fullPath)
		for _, name := range names {
			newAttribute, found := (*entry.Attributes)[name]
			if found {
				oldAttribute, _ := xattr.LGet(fullPath, name)
				if !bytes.Equal(oldAttribute, newAttribute) {
					xattr.LSet(fullPath, name, newAttribute)
				}
				delete(*entry.Attributes, name)
			} else {
				xattr.LRemove(fullPath, name)
			}
		}

		for name, attribute := range *entry.Attributes {
			if len(name) > 0 && name[0] == '\x00' {
				continue
			}
			xattr.LSet(fullPath, name, attribute)
		}
	}
	if err := entry.restoreLateFileFlags(fullPath); err != nil {
		LOG_DEBUG("ATTR_RESTORE", "Could not restore flags for file %s: %v", fullPath, err)
	}
}

func (entry *Entry) RestoreEarlyDirFlags(path string) error {
	return nil
}

func (entry *Entry) RestoreEarlyFileFlags(f *os.File) error {
	return nil
}

func (entry *Entry) readFileFlags(fileInfo os.FileInfo) error {
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if ok && stat.Flags != 0 {
		if entry.Attributes == nil {
			entry.Attributes = &map[string][]byte{}
		}
		v := make([]byte, 4)
		binary.LittleEndian.PutUint32(v, stat.Flags)
		(*entry.Attributes)[bsdFileFlagsKey] = v
		LOG_DEBUG("ATTR_READ", "Read flags 0x%x for %s", stat.Flags, entry.Path)
	}
	return nil
}

func (entry *Entry) restoreLateFileFlags(path string) error {
	if entry.Attributes == nil {
		return nil
	}
	if v, have := (*entry.Attributes)[bsdFileFlagsKey]; have {
		if _, _, errno := syscall.Syscall(syscall.SYS_LCHFLAGS,
			uintptr(unsafe.Pointer(syscall.StringBytePtr(path))),
			uintptr(binary.LittleEndian.Uint32((v))), 0); errno != 0 {
			return os.NewSyscallError("lchflags", errno)
		}
	}
	return nil
}

func (entry *Entry) RestoreSpecial(fullPath string) error {
	mode := entry.Mode & uint32(fileModeMask)

	if entry.Mode&uint32(os.ModeNamedPipe) != 0 {
		mode |= syscall.S_IFIFO
	} else if entry.Mode&uint32(os.ModeCharDevice) != 0 {
		mode |= syscall.S_IFCHR
	} else if entry.Mode&uint32(os.ModeDevice) != 0 {
		mode |= syscall.S_IFBLK
	} else {
		return nil
	}
	return syscall.Mknod(fullPath, mode, entry.GetRdev())
}
