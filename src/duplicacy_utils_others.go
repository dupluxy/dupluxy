// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

//go:build !windows
// +build !windows

package duplicacy

import (
	"fmt"
	"os"
	"path"
	"syscall"

	"golang.org/x/sys/unix"
)

func Readlink(path string) (isRegular bool, s string, err error) {
	s, err = os.Readlink(path)
	return false, s, err
}

func GetOwner(entry *Entry, fileInfo *os.FileInfo) {
	stat, ok := (*fileInfo).Sys().(*syscall.Stat_t)
	if ok && stat != nil {
		entry.UID = int(stat.Uid)
		entry.GID = int(stat.Gid)
	} else {
		entry.UID = -1
		entry.GID = -1
	}
}

func SetOwner(fullPath string, entry *Entry, fileInfo os.FileInfo) bool {
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if ok && stat != nil && (int(stat.Uid) != entry.UID || int(stat.Gid) != entry.GID) {
		if entry.UID != -1 && entry.GID != -1 {
			err := os.Lchown(fullPath, entry.UID, entry.GID)
			if err != nil {
				LOG_ERROR("RESTORE_CHOWN", "Failed to change uid or gid: %v", err)
				return false
			}
		}
	}

	return true
}

type listEntryLinkKey struct {
	dev uint64
	ino uint64
}

func (entry *Entry) getHardLinkKey(f os.FileInfo) (key listEntryLinkKey, linked bool) {
	if entry.IsDir() {
		return
	}
	stat := f.Sys().(*syscall.Stat_t)
	if stat == nil || stat.Nlink < 2 {
		return
	}
	key.dev = uint64(stat.Dev)
	key.ino = uint64(stat.Ino)
	linked = true
	return
}

func (entry *Entry) ReadSpecial(fileInfo os.FileInfo) bool {
	if fileInfo.Mode()&(os.ModeDevice|os.ModeCharDevice) == 0 {
		return true
	}
	stat := fileInfo.Sys().(*syscall.Stat_t)
	if stat == nil {
		return false
	}
	entry.Size = 0
	rdev := uint64(stat.Rdev)
	entry.StartChunk = int(rdev & 0xFFFFFFFF)
	entry.StartOffset = int(rdev >> 32)
	return true
}

func (entry *Entry) GetRdev() uint64 {
	return uint64(entry.StartChunk) | uint64(entry.StartOffset)<<32
}

func (entry *Entry) IsSameSpecial(fileInfo os.FileInfo) bool {
	stat := fileInfo.Sys().(*syscall.Stat_t)
	if stat == nil {
		return false
	}
	return (uint32(fileInfo.Mode()) == entry.Mode) && (uint64(stat.Rdev) == entry.GetRdev())
}

func (entry *Entry) FmtSpecial() string {
	var c string
	mode := entry.Mode & uint32(os.ModeType)

	if mode&uint32(os.ModeNamedPipe) != 0 {
		c = "p"
	} else if mode&uint32(os.ModeCharDevice) != 0 {
		c = "c"
	} else if mode&uint32(os.ModeDevice) != 0 {
		c = "b"
	} else if mode&uint32(os.ModeSocket) != 0 {
		c = "s"
	} else {
		return ""
	}

	rdev := entry.GetRdev()
	return fmt.Sprintf("%s (%d, %d)", c, unix.Major(rdev), unix.Minor(rdev))
}

func MakeHardlink(source string, target string) error {
	return unix.Linkat(unix.AT_FDCWD, source, unix.AT_FDCWD, target, 0)
}

func joinPath(components ...string) string {
	return path.Join(components...)
}

func SplitDir(fullPath string) (dir string, file string) {
	return path.Split(fullPath)
}
