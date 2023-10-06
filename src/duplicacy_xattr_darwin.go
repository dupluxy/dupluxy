// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"syscall"

	"github.com/pkg/xattr"
	"golang.org/x/sys/unix"
)

const (
	darwinFileFlagsKey = "\x00bf"
)

var darwinIsSuperUser bool

func init() {
	darwinIsSuperUser = unix.Geteuid() == 0
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
		(*entry.Attributes)[darwinFileFlagsKey] = v
	}
	return nil
}

func (entry *Entry) SetAttributesToFile(fullPath string) error {
	if entry.Attributes == nil || len(*entry.Attributes) == 0 || entry.IsSpecial() {
		return nil
	}
	attributes := *entry.Attributes

	if _, haveFlags := attributes[darwinFileFlagsKey]; haveFlags && len(attributes) <= 1 {
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
	if entry.Attributes == nil {
		return nil
	}

	if darwinIsSuperUser {
		mask |= ^uint32(unix.UF_SETTABLE | unix.SF_SETTABLE)
	} else {
		mask |= ^uint32(unix.UF_SETTABLE)
	}

	var flags uint32

	if v, have := (*entry.Attributes)[darwinFileFlagsKey]; have {
		flags = binary.LittleEndian.Uint32(v)
	}

	stat := fileInfo.Sys().(*syscall.Stat_t)

	flags = (flags & ^mask) | (stat.Flags & mask)

	if flags != stat.Flags {
		f, err := os.OpenFile(fullPath, os.O_RDONLY|unix.O_SYMLINK, 0)
		if err != nil {
			return err
		}
		err = unix.Fchflags(int(f.Fd()), int(flags))
		f.Close()
		return err
	}
	return nil
}