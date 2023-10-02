// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

//go:build freebsd || netbsd || darwin
// +build freebsd netbsd darwin

package duplicacy

import (
	"encoding/binary"
	"os"
	"syscall"
)

const bsdFileFlagsKey = "\x00bf"

func (entry *Entry) ReadFileFlags(f *os.File) error {
	fileInfo, err := f.Stat()
	if err != nil {
		return err
	}
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

func (entry *Entry) RestoreEarlyDirFlags(path string) error {
	return nil
}

func (entry *Entry) RestoreEarlyFileFlags(f *os.File) error {
	return nil
}

func (entry *Entry) RestoreLateFileFlags(f *os.File) error {
	if entry.Attributes == nil {
		return nil
	}
	if v, have := (*entry.Attributes)[bsdFileFlagsKey]; have {
		LOG_DEBUG("ATTR_RESTORE", "Restore flags 0x%x for %s", binary.LittleEndian.Uint32(v), entry.Path)
		return syscall.Fchflags(int(f.Fd()), int(binary.LittleEndian.Uint32(v)))
	}
	return nil
}
