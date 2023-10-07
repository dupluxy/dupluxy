// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import "os"

func (entry *Entry) readAttributes(fi os.FileInfo, fullPath string, normalize bool) error {
	return nil
}

func (entry *Entry) getFileFlags(fileInfo os.FileInfo) bool {
	return true
}

func (entry *Entry) readFileFlags(fileInfo os.FileInfo, fullPath string) error {
	return nil
}

func (entry *Entry) setAttributesToFile(fullPath string, normalize bool) error {
	return nil
}

func (entry *Entry) restoreEarlyDirFlags(fullPath string, mask uint32) error {
	return nil
}

func (entry *Entry) restoreEarlyFileFlags(f *os.File, mask uint32) error {
	return nil
}

func (entry *Entry) restoreLateFileFlags(fullPath string, fileInfo os.FileInfo, mask uint32) error {
	return nil
}
