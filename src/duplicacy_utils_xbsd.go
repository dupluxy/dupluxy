// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

//go:build freebsd
// +build freebsd

package duplicacy

import (
	"encoding/binary"
	"os"
	"syscall"
)

func excludedByAttribute(attributes map[string][]byte) bool {
	_, excluded := attributes["duplicacy_exclude"]
	if !excluded {
		flags, ok := attributes[bsdFileFlagsKey]
		excluded = ok && (binary.LittleEndian.Uint32(flags)&bsd_UF_NODUMP) != 0
	}
	return excluded
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

type fsId uint64

const invalidFsId fsId = 0

func getFsId(fi os.FileInfo) fsId {
	return fsId(fi.Sys().(*syscall.Stat_t).Dev)
}
