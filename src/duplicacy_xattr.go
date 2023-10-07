// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import "os"

func (entry *Entry) ReadAttributes(fi os.FileInfo, fullPath string, normalize bool) error {
	return entry.readAttributes(fi, fullPath, normalize)
}

func (entry *Entry) GetFileFlags(fileInfo os.FileInfo) bool {
	return entry.getFileFlags(fileInfo)
}

func (entry *Entry) ReadFileFlags(fileInfo os.FileInfo, fullPath string) error {
	return entry.readFileFlags(fileInfo, fullPath)
}

func (entry *Entry) RestoreEarlyDirFlags(fullPath string, mask uint32) error {
	return entry.restoreEarlyDirFlags(fullPath, mask)
}

func (entry *Entry) RestoreEarlyFileFlags(f *os.File, mask uint32) error {
	return entry.restoreEarlyFileFlags(f, mask)
}

func (entry *Entry) RestoreLateFileFlags(fullPath string, fileInfo os.FileInfo, mask uint32) error {
	return entry.restoreLateFileFlags(fullPath, fileInfo, mask)
}

func (entry *Entry) SetAttributesToFile(fullPath string, normalize bool) error {
	return entry.setAttributesToFile(fullPath, normalize)
}
