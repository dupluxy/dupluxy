// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/pkg/xattr"

	"github.com/vmihailenco/msgpack"
)

func TestEntrySort(t *testing.T) {

	DATA := [...]string{
		"ab",
		"ab-",
		"ab0",
		"ab1",
		"\xBB\xDDfile",
		"\xFF\xDDfile",
		"ab/",
		"ab-/",
		"ab0/",
		"ab1/",
		"ab/c",
		"ab+/c-",
		"ab+/c0",
		"ab+/c/",
		"ab+/c+/",
		"ab+/c0/",
		"ab+/c/d",
		"ab+/c+/d",
		"ab+/c0/d",
		"ab-/c",
		"ab1/c",
		"ab1/\xBB\xDDfile",
		"ab1/\xFF\xDDfile",
	}

	var entry1, entry2 *Entry

	for i, p1 := range DATA {
		if p1[len(p1)-1] == '/' {
			entry1 = CreateEntry(p1, 0, 0, 0700|uint32(os.ModeDir))
		} else {
			entry1 = CreateEntry(p1, 0, 0, 0700)
		}
		for j, p2 := range DATA {

			if p2[len(p2)-1] == '/' {
				entry2 = CreateEntry(p2, 0, 0, 0700|uint32(os.ModeDir))
			} else {
				entry2 = CreateEntry(p2, 0, 0, 0700)
			}

			compared := entry1.Compare(entry2)

			if compared < 0 {
				compared = -1
			} else if compared > 0 {
				compared = 1
			}

			var expected int
			if i < j {
				expected = -1
			} else if i > j {
				expected = 1
			} else {
				expected = 0
			}

			if compared != expected {
				t.Errorf("%s vs %s: %d, expected: %d", p1, p2, compared, expected)
			}

		}
	}
}

func TestEntryOrder(t *testing.T) {

	testDir := filepath.Join(os.TempDir(), "duplicacy_test")
	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0700)

	DATA := [...]string{
		"ab",
		"ab-",
		"ab0",
		"ab1",
		"ab+/",
		"ab2/",
		"ab3/",
		"ab+/c",
		"ab+/c+",
		"ab+/c1",
		"ab+/c-/",
		"ab+/c0/",
		"ab+/c-/d",
		"ab+/c0/d",
		"ab2/c",
		"ab3/c",
	}

	var entry1, entry2 *Entry

	for i, p1 := range DATA {
		if p1[len(p1)-1] == '/' {
			entry1 = CreateEntry(p1, 0, 0, 0700|uint32(os.ModeDir))
		} else {
			entry1 = CreateEntry(p1, 0, 0, 0700)
		}
		for j, p2 := range DATA {

			if p2[len(p2)-1] == '/' {
				entry2 = CreateEntry(p2, 0, 0, 0700|uint32(os.ModeDir))
			} else {
				entry2 = CreateEntry(p2, 0, 0, 0700)
			}

			compared := entry1.Compare(entry2)

			if compared < 0 {
				compared = -1
			} else if compared > 0 {
				compared = 1
			}

			var expected int
			if i < j {
				expected = -1
			} else if i > j {
				expected = 1
			} else {
				expected = 0
			}

			if compared != expected {
				t.Errorf("%s vs %s: %d, expected: %d", p1, p2, compared, expected)
			}

		}
	}

	for _, file := range DATA {

		fullPath := filepath.Join(testDir, file)
		if file[len(file)-1] == '/' {
			err := os.Mkdir(fullPath, 0700)
			if err != nil {
				t.Errorf("Mkdir(%s) returned an error: %s", fullPath, err)
			}
			continue
		}

		err := os.WriteFile(fullPath, []byte(file), 0700)
		if err != nil {
			t.Errorf("WriteFile(%s) returned an error: %s", fullPath, err)
		}
	}

	lister := NewLocalDirectoryLister()

	directories := make([]*Entry, 0, 4)
	directories = append(directories, CreateEntry("", 0, 0, 0))

	entries := make([]*Entry, 0, 4)
	entryChannel := make(chan *Entry, 1024)
	entries = append(entries, CreateEntry("", 0, 0, 0))

	for len(directories) > 0 {
		directory := directories[len(directories)-1]
		directories = directories[:len(directories)-1]
		subdirectories, _, err := lister.ListDir(testDir, directory.Path, entryChannel, nil)
		if err != nil {
			t.Errorf("ListEntries(%s, %s) returned an error: %s", testDir, directory.Path, err)
		}
		directories = append(directories, subdirectories...)
	}

	close(entryChannel)
	for entry := range entryChannel {
		entries = append(entries, entry)
	}

	entries = entries[1:]

	for _, entry := range entries {
		t.Logf("entry: %s", entry.Path)
	}

	if len(entries) != len(DATA) {
		t.Errorf("Got %d entries instead of %d", len(entries), len(DATA))
		return
	}

	for i := 0; i < len(entries); i++ {
		if entries[i].Path != DATA[i] {
			t.Errorf("entry: %s, expected: %s", entries[i].Path, DATA[i])
		}
	}

	t.Logf("shuffling %d entries", len(entries))
	for i := range entries {
		j := rand.Intn(i + 1)
		entries[i], entries[j] = entries[j], entries[i]
	}

	sort.Sort(ByName(entries))

	for i := 0; i < len(entries); i++ {
		if entries[i].Path != DATA[i] {
			t.Errorf("entry: %s, expected: %s", entries[i].Path, DATA[i])
		}
	}

	if !t.Failed() {
		os.RemoveAll(testDir)
	}

}

// TestEntryExcludeByAttribute tests the excludeByAttribute parameter to the ListEntries function
func TestEntryExcludeByAttribute(t *testing.T) {

	var excludeAttrName string
	var excludeAttrValue []byte

	if runtime.GOOS == "darwin" {
		excludeAttrName = "com.apple.metadata:com_apple_backup_excludeItem"
		excludeAttrValue = []byte("com.apple.backupd")
	} else if runtime.GOOS == "linux" {
		excludeAttrName = "user.duplicacy_exclude"
	} else if runtime.GOOS == "freebsd" || runtime.GOOS == "netbsd" {
		excludeAttrName = "duplicacy_exclude"
	} else {
		t.Skip("skipping test, not darwin, linux, freebsd, or netbsd")
	}

	testDir := filepath.Join(os.TempDir(), "duplicacy_test")

	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0700)

	// Files or folders named with "exclude" below will have the exclusion attribute set on them
	// When ListEntries is called with excludeByAttribute true, they should be excluded.
	DATA := [...]string{
		"excludefile",
		"includefile",
		"excludedir/",
		"excludedir/file",
		"includedir/",
		"includedir/includefile",
		"includedir/excludefile",
	}

	for _, file := range DATA {
		fullPath := filepath.Join(testDir, file)
		if file[len(file)-1] == '/' {
			err := os.Mkdir(fullPath, 0700)
			if err != nil {
				t.Errorf("Mkdir(%s) returned an error: %s", fullPath, err)
			}
			continue
		}

		err := os.WriteFile(fullPath, []byte(file), 0700)
		if err != nil {
			t.Errorf("WriteFile(%s) returned an error: %s", fullPath, err)
		}
	}

	for _, file := range DATA {
		fullPath := filepath.Join(testDir, file)
		if strings.Contains(file, "exclude") {
			xattr.Set(fullPath, excludeAttrName, excludeAttrValue)
		}
	}

	for _, excludeByAttribute := range [2]bool{true, false} {
		t.Logf("testing excludeByAttribute: %t", excludeByAttribute)

		lister := NewLocalDirectoryLister()
		directories := make([]*Entry, 0, 4)
		directories = append(directories, CreateEntry("", 0, 0, 0))

		entries := make([]*Entry, 0, 4)
		entryChannel := make(chan *Entry, 1024)
		entries = append(entries, CreateEntry("", 0, 0, 0))

		for len(directories) > 0 {
			directory := directories[len(directories)-1]
			directories = directories[:len(directories)-1]
			subdirectories, _, err := lister.ListDir(testDir, directory.Path, entryChannel,
				&EntryListerOptions{
					ExcludeByAttribute: excludeByAttribute,
				})

			if err != nil {
				t.Errorf("ListEntries(%s, %s) returned an error: %s", testDir, directory.Path, err)
			}
			directories = append(directories, subdirectories...)
		}

		close(entryChannel)

		for entry := range entryChannel {
			entries = append(entries, entry)
		}

		entries = entries[1:]

		for _, entry := range entries {
			t.Logf("entry: %s", entry.Path)
		}

		i := 0
		for _, file := range DATA {
			entryFound := false
			var entry *Entry
			for _, entry = range entries {
				if entry.Path == file {
					entryFound = true
					break
				}
			}

			if excludeByAttribute && strings.Contains(file, "exclude") {
				if entryFound {
					t.Errorf("file: %s, expected to be excluded but wasn't. attributes: %v", file, entry.Attributes)
					i++
				} else {
					t.Logf("file: %s, excluded", file)
				}
			} else {
				if entryFound {
					t.Logf("file: %s, included. attributes: %v", file, entry.Attributes)
					i++
				} else {
					t.Errorf("file: %s, expected to be included but wasn't", file)
				}
			}
		}

	}

	if !t.Failed() {
		os.RemoveAll(testDir)
	}

}

func TestEntryEncoding(t *testing.T) {
	buffer := new(bytes.Buffer)
	encoder := msgpack.NewEncoder(buffer)

	entry1 := CreateEntry("abcd", 1, 2, 0700)
	err := encoder.Encode(entry1)
	if err != nil {
		t.Errorf("Failed to encode the entry: %v", err)
		return
	}

	t.Logf("msgpack size: %d\n", len(buffer.Bytes()))
	decoder := msgpack.NewDecoder(buffer)

	description, _ := json.Marshal(entry1)
	t.Logf("json size: %d\n", len(description))

	var entry2 Entry
	err = decoder.Decode(&entry2)
	if err != nil {
		t.Errorf("Failed to decode the entry: %v", err)
		return
	}

	if entry1.Path != entry2.Path || entry1.Size != entry2.Size || entry1.Time != entry2.Time {
		t.Error("Decoded entry is different than the original one")
	}

}
