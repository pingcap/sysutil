// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package sysutil

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
)

type logFile struct {
	file       *os.File // The opened file handle
	begin, end int64    // The timesteamp in millisecond of first line
}

func (l *logFile) BeginTime() int64 {
	return l.begin
}

func (l *logFile) EndTime() int64 {
	return l.end
}

func resolveFiles(logFilePath string, beginTime, endTime int64) ([]logFile, error) {
	if logFilePath == "" {
		return nil, errors.New("empty log file location configuration")
	}

	var logFiles []logFile
	var skipFiles []*os.File
	logDir := filepath.Dir(logFilePath)
	ext := filepath.Ext(logFilePath)
	filePrefix := logFilePath[:len(logFilePath)-len(ext)]
	err := filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// All rotated log files have the same prefix and extension with the original file
		if !strings.HasPrefix(path, filePrefix) {
			return nil
		}
		if !strings.HasSuffix(path, ext) {
			return nil
		}
		// If we cannot open the file, we skip to search the file instead of returning
		// error and abort entire searching task.
		// TODO: do we need to return some warning to client?
		file, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
		if err != nil {
			return nil
		}
		reader := bufio.NewReader(file)
		// Skip this file if cannot read the first line
		firstLine, err := readLine(reader)
		if err != nil && err != io.EOF {
			skipFiles = append(skipFiles, file)
			return nil
		}
		// Skip this file if the first line is not a valid log message
		firstItem, err := parseLogItem(firstLine)
		if err != nil {
			skipFiles = append(skipFiles, file)
			return nil
		}
		// Skip this file if cannot read the last line
		lastLine := readLastLine(file)
		// Skip this file if the last line is not a valid log message
		lastItem, err := parseLogItem(lastLine)
		if err != nil {
			skipFiles = append(skipFiles, file)
			return nil
		}
		// Reset position to the start and skip this file if cannot seek to start
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			skipFiles = append(skipFiles, file)
			return nil
		}
		if beginTime > lastItem.Time || endTime < firstItem.Time {
			skipFiles = append(skipFiles, file)
		} else {
			logFiles = append(logFiles, logFile{
				file:  file,
				begin: firstItem.Time,
				end:   lastItem.Time,
			})
		}
		return nil
	})
	defer func() {
		for _, f := range skipFiles {
			_ = f.Close()
		}
	}()
	// Sort by start time
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].begin < logFiles[j].begin
	})
	return logFiles, err
}

// Read a line from a reader.
func readLine(reader *bufio.Reader) (string, error) {
	var line, b []byte
	var err error
	isPrefix := true
	for isPrefix {
		b, isPrefix, err = reader.ReadLine()
		line = append(line, b...)
		if err != nil {
			return "", err
		}
	}
	return string(line), nil
}

func readLastLine(file *os.File) string {
	var line []byte
	var cursor int64
	stat, _ := file.Stat()
	filesize := stat.Size()
	for {
		cursor -= 1
		file.Seek(cursor, io.SeekEnd)

		char := make([]byte, 1)
		file.Read(char)

		// stop if we find a line
		if cursor != -1 && (char[0] == 10 || char[0] == 13) {
			break
		}
		line = append(line, char[0])
		if cursor == -filesize { // stop if we are at the begining
			break
		}
	}
	for i, j := 0, len(line)-1; i < j; i, j = i+1, j-1 {
		line[i], line[j] = line[j], line[i]
	}
	return string(line)
}

// Returns LogLevel from string and return LogLevel_Info if
// the string is an invalid level string
func ParseLogLevel(s string) pb.LogLevel {
	switch s {
	case "debug", "DEBUG":
		return pb.LogLevel_Debug
	case "info", "INFO":
		return pb.LogLevel_Info
	case "warn", "WARN":
		return pb.LogLevel_Warn
	case "trace", "TRACE":
		return pb.LogLevel_Trace
	case "critical", "CRITICAL":
		return pb.LogLevel_Critical
	case "error", "ERROR":
		return pb.LogLevel_Error
	default:
		return pb.LogLevel_UNKNOWN
	}
}

// parses single log line and returns:
// 1. the timesteamp in unix milliseconds
// 2. the log level
// 3. the log item content
//
// [2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."] ["Release Version"=v2.1.14]...
// [2019/08/26 07:19:49.529 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."] ["Release Version"=v3.0.2]...
// [2019/08/21 01:43:01.460 -04:00] [INFO] [util.go:60] [PD] [release-version=v3.0.2]
// [2019/08/26 07:20:23.815 -04:00] [INFO] [mod.rs:28] ["Release Version:   3.0.2"]
func parseLogItem(s string) (*pb.LogMessage, error) {
	timeLeftBound := strings.Index(s, "[")
	timeRightBound := strings.Index(s, "]")
	if timeLeftBound == -1 || timeRightBound == -1 {
		return nil, fmt.Errorf("invalid log string: %s", s)
	}
	time, err := parseTimeStamp(s[timeLeftBound+1 : timeRightBound])
	if err != nil {
		return nil, err
	}
	levelLeftBound := strings.Index(s[timeRightBound+1:], "[")
	levelRightBound := strings.Index(s[timeRightBound+1:], "]")
	level := ParseLogLevel(s[timeRightBound+1+levelLeftBound+1 : timeRightBound+1+levelRightBound])
	item := &pb.LogMessage{
		Time:    time,
		Level:   level,
		Message: strings.TrimSpace(s[timeRightBound+levelRightBound+2:]),
	}
	return item, nil
}

const TimeStampLayout = "2006/01/02 15:04:05.000 -07:00"

// TiDB / TiKV / PD unified log format
// [2019/03/04 17:04:24.614 +08:00] ...
func parseTimeStamp(s string) (int64, error) {
	t, err := time.Parse(TimeStampLayout, s)
	if err != nil {
		return 0, err
	}
	return t.UnixNano() / int64(time.Millisecond), nil
}

// Only enable seek when position range is more than SEEK_THRESHOLD.
// The suggested value of SEEK_THRESHOLD is the biggest log size.
const SEEK_THRESHOLD = 1024 * 1024

// logIterator implements Iterator and IteratorWithPeek interface.
// It's used for reading logs from log files one by one by their
// time.
type logIterator struct {
	// filters
	begin     int64
	end       int64
	levelFlag int64
	patterns  []*regexp.Regexp

	// inner state
	fileIndex int
	reader    *bufio.Reader
	pending   []*os.File
}

// The Close method close all resources the iterator has.
func (iter *logIterator) close() {
	for _, f := range iter.pending {
		_ = f.Close()
	}
}

func (iter *logIterator) next() (*pb.LogMessage, error) {
	// initial state
	if iter.reader == nil {
		if len(iter.pending) == 0 {
			return nil, io.EOF
		}
		iter.reader = bufio.NewReader(iter.pending[iter.fileIndex])
	}

nextLine:
	for {
		line, err := readLine(iter.reader)
		// Switch to next log file
		if err != nil && err == io.EOF {
			iter.fileIndex++
			if iter.fileIndex >= len(iter.pending) {
				return nil, io.EOF
			}
			iter.reader.Reset(iter.pending[iter.fileIndex])
			continue
		}
		if len(line) < len(TimeStampLayout) {
			continue
		}
		// Skip invalid log item
		item, err := parseLogItem(line)
		if err != nil {
			continue
		}
		if item.Time > iter.end {
			return nil, io.EOF
		}
		if item.Time < iter.begin {
			continue
		}
		if iter.levelFlag != 0 && iter.levelFlag&(1<<item.Level) == 0 {
			continue
		}
		if len(iter.patterns) > 0 {
			for _, p := range iter.patterns {
				if !p.MatchString(item.Message) {
					continue nextLine
				}
			}
		}
		return item, nil
	}
}
