// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/pkg/xattr"
	"golang.org/x/sys/unix"
)

const (
	linux_FS_SECRM_FL        = 0x00000001 /* Secure deletion */
	linux_FS_UNRM_FL         = 0x00000002 /* Undelete */
	linux_FS_COMPR_FL        = 0x00000004 /* Compress file */
	linux_FS_SYNC_FL         = 0x00000008 /* Synchronous updates */
	linux_FS_IMMUTABLE_FL    = 0x00000010 /* Immutable file */
	linux_FS_APPEND_FL       = 0x00000020 /* writes to file may only append */
	linux_FS_NODUMP_FL       = 0x00000040 /* do not dump file */
	linux_FS_NOATIME_FL      = 0x00000080 /* do not update atime */
	linux_FS_NOCOMP_FL       = 0x00000400 /* Don't compress */
	linux_FS_JOURNAL_DATA_FL = 0x00004000 /* Reserved for ext3 */
	linux_FS_NOTAIL_FL       = 0x00008000 /* file tail should not be merged */
	linux_FS_DIRSYNC_FL      = 0x00010000 /* dirsync behaviour (directories only) */
	linux_FS_TOPDIR_FL       = 0x00020000 /* Top of directory hierarchies*/
	linux_FS_NOCOW_FL        = 0x00800000 /* Do not cow file */
	linux_FS_PROJINHERIT_FL  = 0x20000000 /* Create with parents projid */

	linuxIocFlagsFileEarly = linux_FS_SECRM_FL | linux_FS_UNRM_FL | linux_FS_COMPR_FL | linux_FS_NODUMP_FL | linux_FS_NOATIME_FL | linux_FS_NOCOMP_FL | linux_FS_JOURNAL_DATA_FL | linux_FS_NOTAIL_FL | linux_FS_NOCOW_FL
	linuxIocFlagsDirEarly  = linux_FS_TOPDIR_FL | linux_FS_PROJINHERIT_FL
	linuxIocFlagsLate      = linux_FS_SYNC_FL | linux_FS_IMMUTABLE_FL | linux_FS_APPEND_FL | linux_FS_DIRSYNC_FL

	linuxFileFlagsKey = "\x00lf"
)

var (
	errENOTTY error = unix.ENOTTY
)

func ignoringEINTR(fn func() error) (err error) {
	for {
		err = fn()
		if err != unix.EINTR {
			break
		}
	}
	return err
}

func ioctl(f *os.File, request uintptr, attrp *uint32) error {
	return ignoringEINTR(func() error {
		argp := uintptr(unsafe.Pointer(attrp))

		_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), request, argp)
		if errno == 0 {
			return nil
		} else if errno == unix.ENOTTY {
			return errENOTTY
		}
		return errno
	})
}

func (entry *Entry) ReadAttributes(fullPath string, fi os.FileInfo) error {
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
	// the linux file flags interface is quite depressing. The half assed attempt at statx
	// doesn't even cover the flags we're interested in
	if !(entry.IsFile() || entry.IsDir()) {
		return nil
	}

	f, err := os.OpenFile(fullPath, os.O_RDONLY|unix.O_NOATIME|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}

	var flags uint32

	err = ioctl(f, unix.FS_IOC_GETFLAGS, &flags)
	f.Close()
	if err != nil {
		if err == unix.ENOTTY {
			return nil
		}
		return err
	}

	if flags != 0 {
		if entry.Attributes == nil {
			entry.Attributes = &map[string][]byte{}
		}
		v := make([]byte, 4)
		binary.LittleEndian.PutUint32(v, flags)
		(*entry.Attributes)[linuxFileFlagsKey] = v
	}
	return nil
}

func (entry *Entry) SetAttributesToFile(fullPath string, normalize bool) error {
	if entry.Attributes == nil || len(*entry.Attributes) == 0 {
		return nil
	}
	attributes := *entry.Attributes

	if _, haveFlags := attributes[linuxFileFlagsKey]; haveFlags && len(attributes) <= 1 {
		return nil
	}

	names, err := xattr.LList(fullPath)
	if err != nil {
		return err
	}
	for _, name := range names {
		newAttribute, found := (*entry.Attributes)[name]
		if found {
			oldAttribute, _ := xattr.LGet(fullPath, name)
			if !bytes.Equal(oldAttribute, newAttribute) {
				err = errors.Join(err, xattr.LSet(fullPath, name, newAttribute))
			}
			delete(*entry.Attributes, name)
		} else {
			err = errors.Join(err, xattr.LRemove(fullPath, name))
		}
	}

	for name, attribute := range *entry.Attributes {
		if len(name) > 0 && name[0] == '\x00' {
			continue
		}
		err = errors.Join(err, xattr.LSet(fullPath, name, attribute))
	}
	return err
}

func (entry *Entry) RestoreEarlyDirFlags(fullPath string, mask uint32) error {
	if entry.Attributes == nil || mask == 0xffffffff {
		return nil
	}
	var flags uint32

	if v, have := (*entry.Attributes)[linuxFileFlagsKey]; have {
		flags = binary.LittleEndian.Uint32(v) & linuxIocFlagsDirEarly & ^mask
	}

	if flags != 0 {
		f, err := os.OpenFile(fullPath, os.O_RDONLY|unix.O_DIRECTORY, 0)
		if err != nil {
			return err
		}
		err = ioctl(f, unix.FS_IOC_SETFLAGS, &flags)
		f.Close()
		if err != nil {
			return fmt.Errorf("Set flags 0x%.8x failed: %w", flags, err)
		}
	}
	return nil
}

func (entry *Entry) RestoreEarlyFileFlags(f *os.File, mask uint32) error {
	if entry.Attributes == nil || mask == 0xffffffff {
		return nil
	}
	var flags uint32

	if v, have := (*entry.Attributes)[linuxFileFlagsKey]; have {
		flags = binary.LittleEndian.Uint32(v) & linuxIocFlagsFileEarly & ^mask
	}

	if flags != 0 {
		err := ioctl(f, unix.FS_IOC_SETFLAGS, &flags)
		if err != nil {
			return fmt.Errorf("Set flags 0x%.8x failed: %w", flags, err)
		}
	}
	return nil
}

func (entry *Entry) RestoreLateFileFlags(fullPath string, fileInfo os.FileInfo, mask uint32) error {
	if entry.IsLink() || entry.Attributes == nil || mask == 0xffffffff {
		return nil
	}
	var flags uint32

	if v, have := (*entry.Attributes)[linuxFileFlagsKey]; have {
		flags = binary.LittleEndian.Uint32(v) & (linuxIocFlagsFileEarly | linuxIocFlagsDirEarly | linuxIocFlagsLate) & ^mask
	}

	if flags != 0 {
		f, err := os.OpenFile(fullPath, os.O_RDONLY|unix.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		err = ioctl(f, unix.FS_IOC_SETFLAGS, &flags)
		f.Close()
		if err != nil {
			return fmt.Errorf("Set flags 0x%.8x failed: %w", flags, err)
		}
	}
	return nil
}
