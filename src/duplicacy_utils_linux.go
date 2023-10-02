// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"encoding/binary"
	"os"
	"syscall"
	"unsafe"
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

func (entry *Entry) ReadFileFlags(f *os.File) error {
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

func (entry *Entry) RestoreLateFileFlags(f *os.File) error {
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

func excludedByAttribute(attributes map[string][]byte) bool {
	_, ok := attributes["user.duplicacy_exclude"]
	return ok
}
