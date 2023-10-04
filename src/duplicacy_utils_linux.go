// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

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

	linux_FS_IOC_GETFLAGS uintptr = 0x80086601
	linux_FS_IOC_SETFLAGS uintptr = 0x40086602

	linuxIocFlagsFileEarly = linux_FS_SECRM_FL | linux_FS_UNRM_FL | linux_FS_COMPR_FL | linux_FS_NODUMP_FL | linux_FS_NOATIME_FL | linux_FS_NOCOMP_FL | linux_FS_JOURNAL_DATA_FL | linux_FS_NOTAIL_FL | linux_FS_NOCOW_FL
	linuxIocFlagsDirEarly  = linux_FS_TOPDIR_FL | linux_FS_PROJINHERIT_FL
	linuxIocFlagsLate      = linux_FS_SYNC_FL | linux_FS_IMMUTABLE_FL | linux_FS_APPEND_FL | linux_FS_DIRSYNC_FL

	linuxFileFlagsKey = "\x00lf"
)

func ioctl(f *os.File, request uintptr, attrp *uint32) error {
	argp := uintptr(unsafe.Pointer(attrp))

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), request, argp); errno != 0 {
		return os.NewSyscallError("ioctl", errno)
	}
	return nil
}

type xattrHandle struct {
	f        *os.File
	fullPath string
}

func (x xattrHandle) list() ([]string, error) {
	if x.f != nil {
		return xattr.FList(x.f)
	} else {
		return xattr.LList(x.fullPath)
	}
}

func (x xattrHandle) get(name string) ([]byte, error) {
	if x.f != nil {
		return xattr.FGet(x.f, name)
	} else {
		return xattr.LGet(x.fullPath, name)
	}
}

func (x xattrHandle) set(name string, value []byte) error {
	if x.f != nil {
		return xattr.FSet(x.f, name, value)
	} else {
		return xattr.LSet(x.fullPath, name, value)
	}
}

func (x xattrHandle) remove(name string) error {
	if x.f != nil {
		return xattr.FRemove(x.f, name)
	} else {
		return xattr.LRemove(x.fullPath, name)
	}
}

func (entry *Entry) ReadAttributes(top string) {
	fullPath := filepath.Join(top, entry.Path)
	x := xattrHandle{nil, fullPath}

	if !entry.IsLink() {
		var err error
		x.f, err = os.OpenFile(fullPath, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			// FIXME: We really should return errors for failure to read
			return
		}
	}

	attributes, _ := x.list()

	if len(attributes) > 0 {
		entry.Attributes = &map[string][]byte{}
	}
	for _, name := range attributes {
		attribute, err := x.get(name)
		if err == nil {
			(*entry.Attributes)[name] = attribute
		}
	}

	if entry.IsFile() || entry.IsDir() {
		if err := entry.readFileFlags(x.f); err != nil {
			LOG_INFO("ATTR_BACKUP", "Could not backup flags for file %s: %v", fullPath, err)
		}
	}
	x.f.Close()
}

func (entry *Entry) SetAttributesToFile(fullPath string) {
	x := xattrHandle{nil, fullPath}
	if !entry.IsLink() {
		var err error
		x.f, err = os.OpenFile(fullPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return
		}
	}

	names, _ := x.list()

	for _, name := range names {
		newAttribute, found := (*entry.Attributes)[name]
		if found {
			oldAttribute, _ := x.get(name)
			if !bytes.Equal(oldAttribute, newAttribute) {
				x.set(name, newAttribute)
			}
			delete(*entry.Attributes, name)
		} else {
			x.remove(name)
		}
	}

	for name, attribute := range *entry.Attributes {
		if len(name) > 0 && name[0] == '\x00' {
			continue
		}
		x.set(name, attribute)
	}
	if entry.IsFile() || entry.IsDir() {
		if err := entry.restoreLateFileFlags(x.f); err != nil {
			LOG_DEBUG("ATTR_RESTORE", "Could not restore flags for file %s: %v", fullPath, err)
		}
	}
	x.f.Close()
}

func (entry *Entry) readFileFlags(f *os.File) error {
	var flags uint32
	if err := ioctl(f, linux_FS_IOC_GETFLAGS, &flags); err != nil {
		return err
	}
	if flags != 0 {
		if entry.Attributes == nil {
			entry.Attributes = &map[string][]byte{}
		}
		v := make([]byte, 4)
		binary.LittleEndian.PutUint32(v, flags)
		(*entry.Attributes)[linuxFileFlagsKey] = v
		LOG_DEBUG("ATTR_READ", "Read flags 0x%x for %s", flags, entry.Path)
	}
	return nil
}

func (entry *Entry) RestoreEarlyDirFlags(path string) error {
	if entry.Attributes == nil {
		return nil
	}
	if v, have := (*entry.Attributes)[linuxFileFlagsKey]; have {
		flags := binary.LittleEndian.Uint32(v) & linuxIocFlagsDirEarly
		f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_DIRECTORY, 0)
		if err != nil {
			return err
		}
		LOG_DEBUG("ATTR_RESTORE", "Restore dir flags (early) 0x%x for %s", flags, entry.Path)
		err = ioctl(f, linux_FS_IOC_SETFLAGS, &flags)
		f.Close()
		return err
	}
	return nil
}

func (entry *Entry) RestoreEarlyFileFlags(f *os.File) error {
	if entry.Attributes == nil {
		return nil
	}
	if v, have := (*entry.Attributes)[linuxFileFlagsKey]; have {
		flags := binary.LittleEndian.Uint32(v) & linuxIocFlagsFileEarly
		LOG_DEBUG("ATTR_RESTORE", "Restore flags (early) 0x%x for %s", flags, entry.Path)
		return ioctl(f, linux_FS_IOC_SETFLAGS, &flags)
	}
	return nil
}

func (entry *Entry) restoreLateFileFlags(f *os.File) error {
	if entry.Attributes == nil {
		return nil
	}
	if v, have := (*entry.Attributes)[linuxFileFlagsKey]; have {
		flags := binary.LittleEndian.Uint32(v) & (linuxIocFlagsFileEarly | linuxIocFlagsDirEarly | linuxIocFlagsLate)
		LOG_DEBUG("ATTR_RESTORE", "Restore flags (late) 0x%x for %s", flags, entry.Path)
		return ioctl(f, linux_FS_IOC_SETFLAGS, &flags)
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
	} else if entry.Mode&uint32(os.ModeSocket) != 0 {
		mode |= syscall.S_IFSOCK
	} else {
		return nil
	}
	return syscall.Mknod(fullPath, mode, int(entry.GetRdev()))
}

func excludedByAttribute(attributes map[string][]byte) bool {
	_, ok := attributes["user.duplicacy_exclude"]
	return ok
}
