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
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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

func resolveFiles(ctx context.Context, logFilePath string, beginTime, endTime int64) ([]logFile, error) {
	if logFilePath == "" {
		return nil, errors.New("empty log file location configuration")
	}

	var logFiles []logFile
	var skipFiles []*os.File
	logDir := filepath.Dir(logFilePath)
	ext := filepath.Ext(logFilePath)
	filePrefix := logFilePath[:len(logFilePath)-len(ext)]
	files, err := ioutil.ReadDir(logDir)
	if err != nil {
		return nil, err
	}
	walkFn := func(path string, info os.FileInfo) error {
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
		if isCtxDone(ctx) {
			return ctx.Err()
		}
		// If we cannot open the file, we skip to search the file instead of returning
		// error and abort entire searching task.
		// TODO: do we need to return some warning to client?
		file, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
		if err != nil {
			return nil
		}
		reader := bufio.NewReader(file)

		firstItem, err := readFirstValidLog(ctx, reader, 10)
		if err != nil {
			skipFiles = append(skipFiles, file)
			return nil
		}

		lastItem, err := readLastValidLog(ctx, file, 10)
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
	}
	for _, file := range files {
		err := walkFn(filepath.Join(logDir, file.Name()), file)
		if err != nil {
			return nil, err
		}
	}

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

func isCtxDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func readFirstValidLog(ctx context.Context, reader *bufio.Reader, tryLines int64) (*pb.LogMessage, error) {
	var tried int64
	for {
		line, err := readLine(reader)
		if err != nil {
			return nil, err
		}
		item, err := parseLogItem(line)
		if err == nil {
			return item, nil
		}
		tried++
		if tried >= tryLines {
			break
		}
		if isCtxDone(ctx) {
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("not a valid log file")
}

func readLastValidLog(ctx context.Context, file *os.File, tryLines int) (*pb.LogMessage, error) {
	var tried int
	stat, _ := file.Stat()
	endCursor := stat.Size()
	for {
		lines, readBytes, err := readLastLines(ctx, file, endCursor)
		if err != nil {
			return nil, err
		}
		// read out the file
		if readBytes == 0 {
			break
		}
		endCursor -= int64(readBytes)
		for i := len(lines) - 1; i >= 0; i-- {
			item, err := parseLogItem(lines[i])
			if err == nil {
				return item, nil
			}
		}
		tried += len(lines)
		if tried >= tryLines {
			break
		}
	}
	return nil, errors.New("not a valid log file")
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

const maxReadCacheSize = 1024 * 1024 * 16

// Read lines from the end of a file
// endCursor initial value should be the file size
func readLastLines(ctx context.Context, file *os.File, endCursor int64) ([]string, int, error) {
	var lines []byte
	var firstNonNewlinePos int
	var cursor = endCursor
	var size int64 = 256
	for {
		// stop if we are at the begining
		// check it in the start to avoid read beyond the size
		if cursor <= 0 {
			break
		}

		// enlarge the read cache to avoid too many memory move.
		size = size * 2
		if size > maxReadCacheSize {
			size = maxReadCacheSize
		}
		if cursor < size {
			size = cursor
		}
		cursor -= size

		_, err := file.Seek(cursor, io.SeekStart)
		if err != nil {
			return nil, 0, ctx.Err()
		}
		chars := make([]byte, size)
		_, err = file.Read(chars)
		if err != nil {
			return nil, 0, ctx.Err()
		}
		lines = append(chars, lines...)

		// find first '\n' or '\r'
		for i := 0; i < len(chars)-1; i++ {
			// reach the line end
			// the first newline may be in the line end at the first round
			if i >= len(lines)-1 {
				break
			}
			if (chars[i] == 10 || chars[i] == 13) && chars[i+1] != 10 && chars[i+1] != 13 {
				firstNonNewlinePos = i + 1
				break
			}
		}
		if firstNonNewlinePos > 0 {
			break
		}
		if isCtxDone(ctx) {
			return nil, 0, ctx.Err()
		}
	}
	finalStr := string(lines[firstNonNewlinePos:])
	return strings.Split(strings.ReplaceAll(finalStr, "\r\n", "\n"), "\n"), len(finalStr), nil
}

// ParseLogLevel returns LogLevel from string and return LogLevel_Info if
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
	if timeLeftBound == -1 || timeRightBound == -1 || timeLeftBound > timeRightBound {
		return nil, fmt.Errorf("invalid log string: %s", s)
	}
	time, err := parseTimeStamp(s[timeLeftBound+1 : timeRightBound])
	if err != nil {
		return nil, err
	}
	levelLeftBound := strings.Index(s[timeRightBound+1:], "[")
	levelRightBound := strings.Index(s[timeRightBound+1:], "]")
	if levelLeftBound == -1 || levelRightBound == -1 || levelLeftBound > levelRightBound {
		return nil, fmt.Errorf("invalid log string: %s", s)
	}
	level := ParseLogLevel(s[timeRightBound+1+levelLeftBound+1 : timeRightBound+1+levelRightBound])
	item := &pb.LogMessage{
		Time:    time,
		Level:   level,
		Message: strings.TrimSpace(s[timeRightBound+levelRightBound+2:]),
	}
	return item, nil
}

const (
	// TimeStampLayout is accessed in dashboard, keep it public
	TimeStampLayout    = "2006/01/02 15:04:05.000 -07:00"
	timeStampLayoutLen = len(TimeStampLayout)
)

// TiDB / TiKV / PD unified log format
// [2019/03/04 17:04:24.614 +08:00] ...
func parseTimeStamp(s string) (int64, error) {
	t, err := time.Parse(TimeStampLayout, s)
	if err != nil {
		return 0, err
	}
	return t.UnixNano() / int64(time.Millisecond), nil
}

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
	preLog    *pb.LogMessage
}

// The Close method close all resources the iterator has.
func (iter *logIterator) close() {
	for _, f := range iter.pending {
		_ = f.Close()
	}
}

func (iter *logIterator) next(ctx context.Context) (*pb.LogMessage, error) {
	// initial state
	if iter.reader == nil {
		if len(iter.pending) == 0 {
			return nil, io.EOF
		}
		iter.reader = bufio.NewReader(iter.pending[iter.fileIndex])
	}

nextLine:
	for {
		if isCtxDone(ctx) {
			return nil, ctx.Err()
		}
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
		line = strings.TrimSpace(line)
		if iter.preLog == nil && len(line) < timeStampLayoutLen {
			continue
		}
		item, err := parseLogItem(line)
		if err != nil {
			if iter.preLog == nil {
				continue
			}
			// handle invalid log
			// make whole line as log message with pre time and pre log_level
			item = &pb.LogMessage{
				Time:    iter.preLog.Time,
				Level:   iter.preLog.Level,
				Message: line,
			}
		} else {
			iter.preLog = item
		}
		if item.Time > iter.end {
			return nil, io.EOF
		}
		if item.Time < iter.begin {
			continue
		}
		// always keep unknown log_level
		if item.Level > pb.LogLevel_UNKNOWN && iter.levelFlag != 0 && iter.levelFlag&(1<<item.Level) == 0 {
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
