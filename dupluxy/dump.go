// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	duplicacy "github.com/dupluxy/dupluxy/src"
	"github.com/gilbertchen/cli"
)

func dumpAttributes(e *duplicacy.Entry, w io.Writer) {
	//encoder := toml.NewEncoder(&buf)

	if e.Attributes == nil {
		return
	}
	// for name, value := range *e.Attributes {
	// 	encoder.Indent
	// }
	// w.Write(p []byte)
}

func listingDump(context *cli.Context) {
	setGlobalOptions(context)
	defer duplicacy.CatchLogException()

	revision := context.Int("r")
	if revision <= 0 {
		fmt.Fprintf(context.App.Writer, "The revision flag is not specified or invalid\n\n")
		cli.ShowCommandHelp(context, context.Command.Name)
		os.Exit(ArgumentExitCode)
	}

	repository, preference := getRepositoryPreference(context, "")

	duplicacy.LOG_INFO("STORAGE_SET", "Storage set to %s", preference.StorageURL)
	storage := duplicacy.CreateStorage(*preference, false, 1)
	if storage == nil {
		return
	}

	password := ""
	if preference.Encrypted {
		password = duplicacy.GetPassword(*preference, "password", "Enter storage password:", false, false)
	}

	var patterns []string
	for _, pattern := range context.Args() {

		pattern = strings.TrimSpace(pattern)

		for strings.HasPrefix(pattern, "--") {
			pattern = pattern[1:]
		}

		for strings.HasPrefix(pattern, "++") {
			pattern = pattern[1:]
		}

		patterns = append(patterns, pattern)
	}

	patterns = duplicacy.ProcessFilterLines(patterns, make([]string, 0))
	duplicacy.LOG_DEBUG("REGEX_DEBUG", "There are %d compiled regular expressions stored", len(duplicacy.RegexMap))
	duplicacy.LOG_INFO("SNAPSHOT_FILTER", "Loaded %d include/exclude pattern(s)", len(patterns))

	storage.SetRateLimits(context.Int("limit-rate"), 0)

	backupManager := duplicacy.CreateBackupManager(preference.SnapshotID, storage, repository, password, nil)
	backupManager.SetupSnapshotCache(preference.Name)

	loadRSAPrivateKey(context.String("key"), context.String("key-passphrase"), preference, backupManager, false)

	snapshotManager := backupManager.SnapshotManager
	snapshotCache := backupManager.SnapshotCache()
	config := backupManager.Config()

	snapshot := snapshotManager.DownloadSnapshot(preference.SnapshotID, revision)

	if snapshot == nil {
		return
	}

	operator := duplicacy.CreateChunkOperator(config, storage, snapshotCache, context.Bool("stats"), false, 1, false)
	snapshotManager.DownloadSnapshotSequences(snapshot)


	snapshot.ListRemoteFiles(config, operator, func(entry *duplicacy.Entry) bool {
		var buf bytes.Buffer
		err := toml.NewEncoder(&buf).Encode(entry)
		if err != nil {
			duplicacy.LOG_ERROR("SHIT", "Failed: %v", err)
		}
		fmt.Println(buf.String())

		return true
	})

}
