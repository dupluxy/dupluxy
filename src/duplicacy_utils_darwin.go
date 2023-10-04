// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"encoding/binary"
	"os"
	"strings"
	"syscall"
)

func excludedByAttribute(attributes map[string][]byte) bool {
	value, ok := attributes["com.apple.metadata:com_apple_backup_excludeItem"]
	return ok && strings.Contains(string(value), "com.apple.backupd")
}

func (entry *Entry) restoreLateFileFlags(path string) error {
	if entry.Attributes == nil {
		return nil
	}
	if v, have := (*entry.Attributes)[bsdFileFlagsKey]; have {
		f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_SYMLINK, 0)
		if err != nil {
			return err
		}
		err = syscall.Fchflags(int(f.Fd()), int(binary.LittleEndian.Uint32(v)))
		f.Close()
		return err
	}
	return nil
}
