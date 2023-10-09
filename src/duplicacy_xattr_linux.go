// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
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
	linux_FS_CASEFOLD_FL     = 0x40000000 /* Folder is case insensitive */

	linuxIocFlagsFileEarly = linux_FS_SECRM_FL | linux_FS_UNRM_FL | linux_FS_COMPR_FL | linux_FS_NODUMP_FL | linux_FS_NOATIME_FL | linux_FS_NOCOMP_FL | linux_FS_JOURNAL_DATA_FL | linux_FS_NOTAIL_FL | linux_FS_NOCOW_FL
	linuxIocFlagsDirEarly  = linux_FS_TOPDIR_FL | linux_FS_PROJINHERIT_FL | linux_FS_CASEFOLD_FL
	linuxIocFlagsLate      = linux_FS_SYNC_FL | linux_FS_IMMUTABLE_FL | linux_FS_APPEND_FL | linux_FS_DIRSYNC_FL

	linuxFileFlagsKey = "\x00L"
)

func (entry *Entry) readAttributes(fi os.FileInfo, fullPath string, normalize bool) error {
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

func (entry *Entry) getFileFlags(fileInfo os.FileInfo) bool {
	return false
}

func (entry *Entry) readFileFlags(fileInfo os.FileInfo, fullPath string) error {
	// the linux file flags interface is quite depressing. The half assed attempt at statx
	// doesn't even cover the flags we're usually interested in for btrfs
	if !(entry.IsFile() || entry.IsDir()) {
		return nil
	}

	fd, err := openForChFlagsTryNoAtime(fullPath)
	if err != nil {
		return err
	}
	flags, err := ioctlGetUint32Retry(fd, unix.FS_IOC_GETFLAGS)
	closeRetry(fd)

	if err != nil {
		// inappropriate ioctl for device means flags aren't a thing on that FS
		if err == unix.ENOTTY {
			return nil
		}
		return err
	}

	// only store the modifiable flags
	flags &= (linuxIocFlagsDirEarly | linuxIocFlagsFileEarly | linuxIocFlagsDirEarly)

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

func (entry *Entry) setAttributesToFile(fullPath string, normalize bool) error {
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

func (entry *Entry) restoreEarlyDirFlags(fullPath string, mask uint32) error {
	if entry.Attributes == nil || mask == math.MaxUint32 {
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
		err = ioctlSetUint32Retry(int(f.Fd()), unix.FS_IOC_SETFLAGS, flags)
		f.Close()
		if err != nil {
			return fmt.Errorf("Set flags 0x%.8x failed: %w", flags, err)
		}
	}
	return nil
}

func (entry *Entry) restoreEarlyFileFlags(f *os.File, mask uint32) error {
	if entry.Attributes == nil || mask == math.MaxUint32 {
		return nil
	}
	var flags uint32

	if v, have := (*entry.Attributes)[linuxFileFlagsKey]; have {
		flags = binary.LittleEndian.Uint32(v) & linuxIocFlagsFileEarly & ^mask
	}

	if flags != 0 {
		err := ioctlSetUint32Retry(int(f.Fd()), unix.FS_IOC_SETFLAGS, flags)
		if err != nil {
			return fmt.Errorf("Set flags 0x%.8x failed: %w", flags, err)
		}
	}
	return nil
}

func (entry *Entry) restoreLateFileFlags(fullPath string, fileInfo os.FileInfo, mask uint32) error {
	if entry.IsLink() || entry.Attributes == nil || mask == math.MaxUint32 {
		return nil
	}
	var flags uint32

	if v, have := (*entry.Attributes)[linuxFileFlagsKey]; have {
		flags = binary.LittleEndian.Uint32(v) & (linuxIocFlagsFileEarly | linuxIocFlagsDirEarly | linuxIocFlagsLate) & ^mask
	}

	if flags != 0 {
		fd, err := openForChFlagsTryNoAtime(fullPath)
		if err != nil {
			return err
		}
		err = ioctlSetUint32Retry(fd, unix.FS_IOC_SETFLAGS, flags)
		closeRetry(fd)
		if err != nil {
			return fmt.Errorf("Set flags 0x%.8x failed: %w", flags, err)
		}
	}
	return nil
}

func ioctlGetUint32Retry(fd int, req uint) (uint32, error) {
	return ignoringEINTRUint32(func() (value uint32, err error) {
		argp := uintptr(unsafe.Pointer(&value))
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), argp)
		if errno != 0 {
			err = errno
		}
		return
	})
}

func ioctlSetUint32Retry(fd int, req uint, val uint32) error {
	return ignoringEINTR(func() error {
		argp := uintptr(unsafe.Pointer(&val))
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), argp)
		if errno == 0 {
			return nil
		}
		return errno
	})
}

func openForChFlagsTryNoAtime(path string) (fd int, err error) {
	fd, err = ignoringEINTRInt(func() (int, error) {
		return unix.Open(path, unix.O_NOATIME|unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	})
	if err == unix.EPERM {
		fd, err = ignoringEINTRInt(func() (int, error) {
			return unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
		})
	}
	return
}

func closeRetry(fd int) error {
	return ignoringEINTR(func() error {
		return unix.Close(fd)
	})
}

func ignoringEINTR(fn func() error) (err error) {
	for {
		err = fn()
		if err != unix.EINTR {
			break
		}
	}
	return
}

func ignoringEINTRInt(fn func() (int, error)) (r int, err error) {
	for {
		r, err = fn()
		if err != unix.EINTR {
			break
		}
	}
	return
}

func ignoringEINTRUint32(fn func() (uint32, error)) (r uint32, err error) {
	for {
		r, err = fn()
		if err != unix.EINTR {
			break
		}
	}
	return
}
