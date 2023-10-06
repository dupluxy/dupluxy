// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"encoding/binary"
	"os"

	"golang.org/x/sys/unix"
)

func excludedByAttribute(attributes map[string][]byte) bool {
	_, excluded := attributes["user.duplicacy_exclude"]
	if !excluded {
		flags, ok := attributes[linuxFileFlagsKey]
		excluded = ok && (binary.LittleEndian.Uint32(flags)&linux_FS_NODUMP_FL) != 0
	}
	return excluded
}

func (entry *Entry) RestoreSpecial(fullPath string) error {
	mode := entry.Mode & uint32(fileModeMask)

	if entry.Mode&uint32(os.ModeNamedPipe) != 0 {
		mode |= unix.S_IFIFO
	} else if entry.Mode&uint32(os.ModeCharDevice) != 0 {
		mode |= unix.S_IFCHR
	} else if entry.Mode&uint32(os.ModeDevice) != 0 {
		mode |= unix.S_IFBLK
	} else if entry.Mode&uint32(os.ModeSocket) != 0 {
		mode |= unix.S_IFSOCK
	} else {
		return nil
	}
	return unix.Mknod(fullPath, mode, int(entry.GetRdev()))
}
