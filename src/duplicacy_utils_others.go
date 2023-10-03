// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

// +build !windows

package duplicacy

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"syscall"

	"github.com/pkg/xattr"
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

func SetOwner(fullPath string, entry *Entry, fileInfo *os.FileInfo) bool {
	stat, ok := (*fileInfo).Sys().(*syscall.Stat_t)
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

func (entry *Entry) ReadAttributes(top string) {
	fullPath := filepath.Join(top, entry.Path)
	f, err := os.OpenFile(fullPath, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	attributes, _ := xattr.FList(f)
	if len(attributes) > 0 {
		entry.Attributes = &map[string][]byte{}
		for _, name := range attributes {
			attribute, err := xattr.Get(fullPath, name)
			if err == nil {
				(*entry.Attributes)[name] = attribute
			}
		}
	}
	if err := entry.ReadFileFlags(f); err != nil {
		LOG_INFO("ATTR_BACKUP", "Could not backup flags for file %s: %v", fullPath, err)
	}
	f.Close()
}

func (entry *Entry) SetAttributesToFile(fullPath string) {
	f, err := os.OpenFile(fullPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return
	}

	names, _ := xattr.FList(f)
	for _, name := range names {
		newAttribute, found := (*entry.Attributes)[name]
		if found {
			oldAttribute, _ := xattr.FGet(f, name)
			if !bytes.Equal(oldAttribute, newAttribute) {
				xattr.FSet(f, name, newAttribute)
			}
			delete(*entry.Attributes, name)
		} else {
			xattr.FRemove(f, name)
		}
	}

	for name, attribute := range *entry.Attributes {
		if len(name) > 0 && name[0] == '\x00' {
			continue
		}
		xattr.FSet(f, name, attribute)
	}
	if err := entry.RestoreLateFileFlags(f); err != nil {
		LOG_DEBUG("ATTR_RESTORE", "Could not restore flags for file %s: %v", fullPath, err)
	}
	f.Close()
}

func (entry *Entry) ReadSpecial(fileInfo os.FileInfo) bool {
	if fileInfo.Mode() & (os.ModeDevice | os.ModeCharDevice) == 0 {
		return true
	}
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return false
	}
	entry.Size = 0
	rdev := uint64(stat.Rdev)
	entry.StartChunk = int(rdev & 0xFFFFFFFF)
	entry.StartOffset = int(rdev >> 32)
	return true
}

func (entry *Entry) RestoreSpecial(fullPath string) error {
	if entry.Mode & uint32(os.ModeDevice | os.ModeCharDevice) != 0 {
		mode := entry.Mode & uint32(fileModeMask)
		if entry.Mode & uint32(os.ModeCharDevice) != 0 {
			mode |= syscall.S_IFCHR
		} else {
			mode |= syscall.S_IFBLK
		}
		rdev := uint64(entry.StartChunk) | uint64(entry.StartOffset) << 32
		return syscall.Mknod(fullPath, mode, int(rdev))
	} else if entry.Mode & uint32(os.ModeNamedPipe) != 0 {
		return syscall.Mkfifo(fullPath, uint32(entry.Mode))
	}
	return nil
}

func joinPath(components ...string) string {
	return path.Join(components...)
}

func SplitDir(fullPath string) (dir string, file string) {
	return path.Split(fullPath)
}
