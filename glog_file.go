// Go support for leveled logs, analogous to https://code.google.com/p/google-glog/
//
// Copyright 2013 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// File I/O for logs.

package glog

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MaxSize is the maximum size of a log file in bytes.
var MaxSize uint64 = 1024 * 1024 * 16
var MaxFileCount int = 4

// logDirs lists the candidate directories for new log files.
var logDirs []string

// If non-empty, overrides the choice of directory in which to write logs.
// See createLogDirs for the full list of possible destinations.
var logDir = flag.String("log_dir", "", "If non-empty, write log files in this directory")

func createLogDirs() {
	if *logDir != "" {
		logDirs = append(logDirs, *logDir)
	}
	logDirs = append(logDirs, os.TempDir())
}

var (
	pid      = os.Getpid()
	program  = filepath.Base(os.Args[0])
	host     = "unknownhost"
	userName = "unknownuser"
)

func init() {
	h, err := os.Hostname()
	if err == nil {
		host = shortHostname(h)
	}

	current, err := user.Current()
	if err == nil {
		userName = current.Username
	}

	// Sanitize userName since it may contain filepath separators on Windows.
	userName = strings.Replace(userName, `\`, "_", -1)
}

// shortHostname returns its argument, truncating at the first period.
// For instance, given "www.google.com" it returns "www".
func shortHostname(hostname string) string {
	if i := strings.Index(hostname, "."); i >= 0 {
		return hostname[:i]
	}
	return hostname
}

// logName returns a new log file name containing tag, with start time t, and
// the name for the symlink for tag.
func logName(tag string, t time.Time) (name, link string) {
	name = fmt.Sprintf("%s[%04d-%02d-%02d %02d-%02d-%02d].log",
		program[:len(program)-len(filepath.Ext(program))],
		t.Year(),
		t.Month(),
		t.Day(),
		t.Hour(),
		t.Minute(),
		t.Second())
	return name, program + "." + tag
}

func logPrefix(tag string) string {
	return program[:len(program)-len(filepath.Ext(program))]
}

var onceLogDirs sync.Once

// create creates a new log file and returns the file and its filename, which
// contains tag ("INFO", "FATAL", etc.) and t.  If the file is created
// successfully, create also attempts to update the symlink for that tag, ignoring
// errors.
func create(tag string, t time.Time) (f *os.File, filename string, err error) {
	onceLogDirs.Do(createLogDirs)
	if len(logDirs) == 0 {
		return nil, "", errors.New("log: no log dirs")
	}
	name, _ := logName(tag, t)
	var lastErr error
	for _, dir := range logDirs {
		fname := filepath.Join(dir, name)
		f, err := os.Create(fname)
		if err == nil {
			return f, fname, nil
		}
		lastErr = err
	}
	return nil, "", fmt.Errorf("log: cannot create log: %v", lastErr)
}

func deleteOldLogFile(tag string, maxFileCount int) (count int, err error) {
	onceLogDirs.Do(createLogDirs)
	if len(logDirs) == 0 {
		return 0, errors.New("log: no log dirs")
	}
	prefix := logPrefix(tag)

	logFileCount := 0
	var fileToDelete os.FileInfo
	var foundDir = false
	var logDir string

	for _, dir := range logDirs {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			continue
		}

		if foundDir {
			break
		}

		for _, file := range files {
			if !file.Mode().IsRegular() {
				continue
			}
			if strings.HasPrefix(file.Name(), prefix) {
				foundDir = true
				logDir = dir
				logFileCount++

				if fileToDelete == nil {
					fileToDelete = file
				} else {
					if fileToDelete.ModTime().After(file.ModTime()) {
						fileToDelete = file
					}
				}
			}
		}
	}

	if logFileCount > maxFileCount {
		if fileToDelete != nil {
			err := os.Remove(filepath.Join(logDir, fileToDelete.Name()))
			if err != nil {
				return logFileCount, err
			}
		}
	}

	return logFileCount, nil
}
