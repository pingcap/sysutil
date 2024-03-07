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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sysutil_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/pingcap/sysutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type searchLogSuite struct {
	server  *grpc.Server
	address string
	tmpDir  string
}

func createSearchLogSuite(t testing.TB) (*searchLogSuite, func()) {
	tmpDir, err := ioutil.TempDir("", "sysutil")
	require.NoError(t, err)

	server := grpc.NewServer()
	pb.RegisterDiagnosticsServer(server, sysutil.NewDiagnosticsServer(filepath.Join(tmpDir, "rpc.tidb.log")))

	// Find a available port
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "cannot find available port")

	s := new(searchLogSuite)
	s.tmpDir = tmpDir
	s.server = server
	s.address = fmt.Sprintf(":%d", listener.Addr().(*net.TCPAddr).Port)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	return s, func() {
		s.server.Stop()
		require.NoError(t, os.RemoveAll(s.tmpDir), fmt.Sprintf("remote tmpDir %v failed", s.tmpDir))
	}
}

func (s *searchLogSuite) writeTmpFile(t testing.TB, filename string, lines []string) {
	err := ioutil.WriteFile(filepath.Join(s.tmpDir, filename), []byte(strings.Join(lines, "\n")), os.ModePerm)
	require.NoError(t, err, fmt.Sprintf("write tmp file %s failed", filename))
}

func (s *searchLogSuite) writeTmpGzipFile(t testing.TB, filename string, lines []string) {
	gzf, err := os.OpenFile(filepath.Join(s.tmpDir, filename), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.ModePerm)
	require.NoError(t, err, fmt.Sprintf("write tmp gzip file %s failed", filename))
	gz := gzip.NewWriter(gzf)
	defer gz.Close()
	_, err = gz.Write([]byte(strings.Join(lines, "\n")))
	require.NoError(t, err, fmt.Sprintf("write tmp gzip file %s failed", filename))
}

func TestResolveFiles(t *testing.T) {
	s, clean := createSearchLogSuite(t)
	defer clean()

	s.writeTmpFile(t, "tidb.log", []string{
		`20/08/26 06:19:13.011 -04:00 [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`hello TiDB`,
		`[2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`20/08/26 06:19:13.011 -04:00 [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`hello TiDB`,
	})

	// single line file
	s.writeTmpFile(t, "tidb-1.log", []string{
		`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// two lines file
	s.writeTmpFile(t, "tidb-2.log", []string{
		`[2019/08/26 06:21:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// empty file
	s.writeTmpFile(t, "tidb-3.log", []string{``})
	s.writeTmpFile(t, "tidb-4.log", []string{
		`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	type timeRange struct{ start, end string }
	cases := []struct {
		search timeRange
		expect []timeRange
	}{
		// 0
		// all files
		{
			search: timeRange{"2019/08/26 06:19:13.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			expect: []timeRange{
				{"2019/08/26 06:19:13.011 -04:00", "2019/08/26 06:19:17.011 -04:00"},
				{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:20:14.011 -04:00"},
				{"2019/08/26 06:21:14.011 -04:00", "2019/08/26 06:21:15.011 -04:00"},
				{"2019/08/26 06:22:14.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			},
		},
		// 1
		// emptys
		{
			search: timeRange{"2019/08/26 06:29:13.011 -04:00", "2019/08/26 06:32:17.011 -04:00"},
			expect: []timeRange{},
		},
		// 2
		// single line
		{
			search: timeRange{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:20:14.011 -04:00"},
			expect: []timeRange{
				{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:20:14.011 -04:00"},
			},
		},
		// 3
		{
			search: timeRange{"2019/08/26 06:21:14.011 -04:00", "2019/08/26 06:21:15.011 -04:00"},
			expect: []timeRange{
				{"2019/08/26 06:21:14.011 -04:00", "2019/08/26 06:21:15.011 -04:00"},
			},
		},
		// 4
		{
			search: timeRange{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:21:15.011 -04:00"},
			expect: []timeRange{
				{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:20:14.011 -04:00"},
				{"2019/08/26 06:21:14.011 -04:00", "2019/08/26 06:21:15.011 -04:00"},
			},
		},
		// 5
		{
			search: timeRange{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			expect: []timeRange{
				{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:20:14.011 -04:00"},
				{"2019/08/26 06:21:14.011 -04:00", "2019/08/26 06:21:15.011 -04:00"},
				{"2019/08/26 06:22:14.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			},
		},
		// 6
		{
			search: timeRange{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:22:15.011 -04:00"},
			expect: []timeRange{
				{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:20:14.011 -04:00"},
				{"2019/08/26 06:21:14.011 -04:00", "2019/08/26 06:21:15.011 -04:00"},
				{"2019/08/26 06:22:14.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			},
		},
		// 7
		{
			search: timeRange{"2019/08/26 06:19:15.011 -04:00", "2019/08/26 06:22:15.011 -04:00"},
			expect: []timeRange{
				{"2019/08/26 06:19:13.011 -04:00", "2019/08/26 06:19:17.011 -04:00"},
				{"2019/08/26 06:20:14.011 -04:00", "2019/08/26 06:20:14.011 -04:00"},
				{"2019/08/26 06:21:14.011 -04:00", "2019/08/26 06:21:15.011 -04:00"},
				{"2019/08/26 06:22:14.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			},
		},
		// 8
		{
			search: timeRange{"2019/08/26 06:22:14.011 -04:00", "2019/08/26 06:22:16.011 -04:00"},
			expect: []timeRange{
				{"2019/08/26 06:22:14.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			},
		},
	}

	for i, cas := range cases {
		beginTime, err := sysutil.ParseTimeStamp(cas.search.start)
		require.NoError(t, err)
		endTime, err := sysutil.ParseTimeStamp(cas.search.end)
		require.NoError(t, err)
		logFiles, err := sysutil.ResolveFiles(context.Background(), filepath.Join(s.tmpDir, "tidb.log"), beginTime, endTime)
		require.NoError(t, err)
		require.Len(t, logFiles, len(cas.expect), fmt.Sprintf("search range (index: %d): %+v", i, cas.search))

		for j, exp := range cas.expect {
			beginTime, err := sysutil.ParseTimeStamp(exp.start)
			require.NoError(t, err)
			endTime, err := sysutil.ParseTimeStamp(exp.end)
			require.NoError(t, err)
			require.Equal(t, logFiles[j].BeginTime(), beginTime, fmt.Sprintf("case index: %d, expect index: %v", i, j))
			require.Equal(t, logFiles[j].EndTime(), endTime, fmt.Sprintf("case index: %d, expect index: %v", i, j))
		}
	}
}

func TestLogIterator(t *testing.T) {
	s, clean := createSearchLogSuite(t)
	defer clean()

	s.writeTmpFile(t, "rpc.tidb.log", []string{
		`[2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
		`This is an invalid log blablabla][`,
		`[2019/08/26 06:19:17.011 -04:00] ] [INFO] invalid log"]`,
		`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// single line file
	s.writeTmpFile(t, "rpc.tidb-1.log", []string{
		`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// two lines file
	s.writeTmpFile(t, "rpc.tidb-2.log", []string{
		`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// empty file
	s.writeTmpFile(t, "rpc.tidb-3.log", []string{``})
	s.writeTmpFile(t, "rpc.tidb-4.log", []string{
		`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`This is also an invalid log contains partern ...`,
		`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
	})
	s.writeTmpFile(t, "rpc.tidb-5.log", []string{
		`[2019/08/26 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test to TiDB."]`,
		`[2019/08/27 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test txn to TiDB."]`,
	})

	type timeRange struct{ start, end string }

	// filter by time range
	cases := []struct {
		search   timeRange
		expect   []string
		levels   []pb.LogLevel
		patterns []string
	}{
		// 0
		{
			search: timeRange{"2019/08/26 06:19:13.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] This is an invalid log blablabla][`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [2019/08/26 06:19:17.011 -04:00] ] [INFO] invalid log"]`,
				`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] This is also an invalid log contains partern ...`,
				`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		// 1
		{
			search: timeRange{"2000/08/26 06:19:13.011 -04:00", "2099/08/26 06:22:17.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] This is an invalid log blablabla][`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [2019/08/26 06:19:17.011 -04:00] ] [INFO] invalid log"]`,
				`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] This is also an invalid log contains partern ...`,
				`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test to TiDB."]`,
				`[2019/08/27 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test txn to TiDB."]`,
			},
		},
		// 2
		{
			search: timeRange{"2000/08/26 06:19:13.011 -04:00", "2019/08/26 06:19:17.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] This is an invalid log blablabla][`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [2019/08/26 06:19:17.011 -04:00] ] [INFO] invalid log"]`,
				`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		// 3
		{
			search: timeRange{"2019/08/26 06:20:14.011 -04:00", "2099/08/26 06:22:17.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] This is also an invalid log contains partern ...`,
				`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test to TiDB."]`,
				`[2019/08/27 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test txn to TiDB."]`,
			},
		},
		// 4
		{
			search: timeRange{"2019/08/26 06:19:15.011 -04:00", "2019/08/26 06:22:14.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] This is an invalid log blablabla][`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [2019/08/26 06:19:17.011 -04:00] ] [INFO] invalid log"]`,
				`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] This is also an invalid log contains partern ...`,
				`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		// 5
		{
			search: timeRange{"2019/08/26 06:19:14.011 -04:00", "2019/08/26 06:19:15.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		// 6
		{
			search: timeRange{"2019/08/26 06:19:14.011 -04:00", "2019/08/26 06:19:16.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] This is an invalid log blablabla][`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [2019/08/26 06:19:17.011 -04:00] ] [INFO] invalid log"]`,
			},
		},
		// 7
		{
			search: timeRange{"2019/08/26 06:19:14.011 -04:00", "2019/08/26 06:19:16.012 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] This is an invalid log blablabla][`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [2019/08/26 06:19:17.011 -04:00] ] [INFO] invalid log"]`,
			},
		},
		// 8
		{
			search: timeRange{"2019/08/26 06:19:15.011 -04:00", "2019/08/26 06:22:14.011 -04:00"},
			levels: []pb.LogLevel{pb.LogLevel_Debug},
			expect: []string{
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] This is an invalid log blablabla][`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [2019/08/26 06:19:17.011 -04:00] ] [INFO] invalid log"]`,
			},
		},
		// 9
		{
			search: timeRange{"2019/08/26 06:19:15.011 -04:00", "2019/08/26 06:22:14.011 -04:00"},
			levels: []pb.LogLevel{pb.LogLevel_Trace},
			expect: []string{
				`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		// 10
		{
			search: timeRange{"2019/08/26 06:19:15.011 -04:00", "2019/08/26 06:22:14.011 -04:00"},
			levels: []pb.LogLevel{pb.LogLevel_Info},
			expect: []string{
				`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] This is also an invalid log contains partern ...`,
			},
		},
		// 11
		{
			search: timeRange{"2019/08/26 06:19:15.011 -04:00", "2019/08/26 06:22:14.011 -04:00"},
			levels: []pb.LogLevel{pb.LogLevel_Warn},
			expect: []string{
				`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		// 12
		{
			search: timeRange{"2019/08/26 06:19:14.011 -04:00", "2019/08/26 06:19:16.012 -04:00"},
			levels: []pb.LogLevel{pb.LogLevel_Error},
			expect: []string{
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		// 13
		{
			search:   timeRange{"2019/08/26 06:19:14.011 -04:00", "2019/08/27 06:23:17.011 -04:00"},
			levels:   []pb.LogLevel{pb.LogLevel_Info},
			patterns: []string{".*partern.*"},
			expect: []string{
				`[2019/08/26 06:22:14.011 -04:00] [INFO] This is also an invalid log contains partern ...`,
				`[2019/08/26 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test to TiDB."]`,
				`[2019/08/27 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test txn to TiDB."]`,
			},
		},
		// 14
		{
			search:   timeRange{"2019/08/26 06:19:14.011 -04:00", "2019/08/27 06:23:17.011 -04:00"},
			levels:   []pb.LogLevel{pb.LogLevel_Info},
			patterns: []string{".*partern.*", ".*txn.*"},
			expect: []string{
				`[2019/08/27 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test txn to TiDB."]`,
			},
		},
	}

	// Set up a connection to the server.
	conn, err := grpc.Dial(s.address, grpc.WithInsecure())
	require.NoError(t, err)

	defer func() {
		require.NoError(t, conn.Close())
	}()

	for i, cas := range cases {
		func() {
			beginTime, err := sysutil.ParseTimeStamp(cas.search.start)
			require.NoError(t, err)
			endTime, err := sysutil.ParseTimeStamp(cas.search.end)
			require.NoError(t, err)
			req := &pb.SearchLogRequest{
				StartTime: beginTime,
				EndTime:   endTime,
				Levels:    cas.levels,
				Patterns:  cas.patterns,
			}
			client := pb.NewDiagnosticsClient(conn)

			// Contact the server and print out its response.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			stream, err := client.SearchLog(ctx, req)
			require.NoError(t, err)

			resp := &pb.SearchLogResponse{}
			for {
				res, err := stream.Recv()
				if err != nil && err == io.EOF {
					break
				}
				require.NoError(t, err)
				require.NotNil(t, res)
				resp.Messages = append(resp.Messages, res.Messages...)
			}

			var items []*pb.LogMessage
			for _, s := range cas.expect {
				item, err := sysutil.ParseLogItem(s)
				require.NoError(t, err)
				items = append(items, item)
			}
			require.Equal(t, len(items), len(resp.Messages), fmt.Sprintf("search log (index: %d) failed", i))
			require.Equal(t, items, resp.Messages, fmt.Sprintf("search log (index: %d) failed", i))
		}()
	}
}

func TestParseLogLevel(t *testing.T) {
	cases := []struct {
		s string
		l pb.LogLevel
	}{
		{"debug", pb.LogLevel_Debug},
		{"DEBUG", pb.LogLevel_Debug},
		{"info", pb.LogLevel_Info},
		{"INFO", pb.LogLevel_Info},
		{"warn", pb.LogLevel_Warn},
		{"WARN", pb.LogLevel_Warn},
		{"trace", pb.LogLevel_Trace},
		{"TRACE", pb.LogLevel_Trace},
		{"critical", pb.LogLevel_Critical},
		{"CRITICAL", pb.LogLevel_Critical},
		{"error", pb.LogLevel_Error},
		{"ERROR", pb.LogLevel_Error},
		{"invalid", pb.LogLevel_UNKNOWN},
	}

	for _, cas := range cases {
		got := sysutil.ParseLogLevel(cas.s)
		require.Equal(t, cas.l, got, fmt.Sprintf("parse %v, expected: %v, got: %v", cas.s, cas.l, got))
	}
}

func TestParseLogItem(t *testing.T) {
	cases := []struct {
		raw     string
		time    string
		level   pb.LogLevel
		message string
	}{
		{
			raw:     `[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
			time:    `2019/08/26 06:19:15.011 -04:00`,
			level:   pb.LogLevel_Error,
			message: `[printer.go:41] ["Welcome to TiDB."]`,
		},
	}

	for _, cas := range cases {
		item, err := sysutil.ParseLogItem(cas.raw)
		require.NoError(t, err)
		require.Equal(t, cas.level, item.Level)
		tt, err := sysutil.ParseTimeStamp(cas.time)
		require.NoError(t, err)
		require.Equal(t, tt, item.Time)
		require.Equal(t, cas.message, item.Message)
	}
}

func TestReadLastLinesHuge(t *testing.T) {
	s, clean := createSearchLogSuite(t)
	defer clean()

	// step 1. initial a log file with lastHugeLine
	lastHugeLine := make([]byte, 0, 1024+512)
	lastHugeLine = append(lastHugeLine, []byte(`[2019/08/26 06:22:20.011 -04:00] [INFO] [printer.go:41] `)...)
	for len(lastHugeLine) < cap(lastHugeLine) {
		if len(lastHugeLine) == 1023 || len(lastHugeLine) == cap(lastHugeLine)-1 {
			lastHugeLine = append(lastHugeLine, '\n')
		} else {
			lastHugeLine = append(lastHugeLine, 'a')
		}
	}
	s.writeTmpFile(t, "tidb.log", []string{
		`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		string(lastHugeLine),
	})

	// step 2. open it as read only mode
	path := filepath.Join(s.tmpDir, "tidb.log")
	file, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	require.NoError(t, err, fmt.Sprintf("open file %s failed", path))
	defer func() {
		require.NoError(t, file.Close())
	}()

	stat, err := file.Stat()
	require.NoError(t, err)
	lastLines, _, err := sysutil.ReadLastLines(context.Background(), file, stat.Size())
	require.NoError(t, err)
	require.Len(t, lastLines, 4)
}

func TestReadAndAppendLogFile(t *testing.T) {
	s, clean := createSearchLogSuite(t)
	defer clean()

	// step 1. initial a log file
	s.writeTmpFile(t, "tidb.log", []string{
		`[2019/08/26 06:22:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]` + "\n",
	})

	// step 2. open it as read only mode
	path := filepath.Join(s.tmpDir, "tidb.log")
	file, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	require.NoError(t, err, fmt.Sprintf("open file %s failed", path))
	defer func() {
		require.NoError(t, file.Close())
	}()

	stat, _ := file.Stat()
	filesize := stat.Size()

	// step 3. run a goroutine to append some new logs to the file
	go func() {
		fileAppend, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
		require.NoError(t, err, fmt.Sprintf("open file as append mode %s failed", path))
		defer func() {
			require.NoError(t, fileAppend.Close())
		}()

		for i := 40; i < 45; i++ {
			line := fmt.Sprintf("[2020/07/07 06:%d:17.011 -04:00] [INFO] appended logs\n", i)
			_, err := fileAppend.WriteString(line)
			require.NoError(t, err, fmt.Sprintf("write %s failed", line))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// step 4. keep calling ReadLastLines until reach the beginning
	expected := []string{
		`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]` + "\n" +
			`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]` + "\n" +
			`[2019/08/26 06:22:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]` + "\n" +
			`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]` + "\n",
		`[2019/08/26 06:22:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]` + "\n",
	}
	i := 0
	endCursor := filesize
	for {
		lines, readBytes, _ := sysutil.ReadLastLines(context.Background(), file, endCursor)
		// read out the file
		if readBytes == 0 {
			break
		}
		require.Equal(t, expected[i], strings.Join(lines, "\n"), fmt.Sprintf("expected: %v, got: %v", expected[i], lines))
		i++
		endCursor -= int64(readBytes)

		time.Sleep(15 * time.Millisecond)
	}
}

func TestCompressLog(t *testing.T) {
	s, clean := createSearchLogSuite(t)
	defer clean()

	s.writeTmpGzipFile(t, "rpc.tidb-2.log.gz", []string{
		`[2019/08/26 06:22:08.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:09.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:10.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:11.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:12.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	s.writeTmpGzipFile(t, "rpc.tidb-1.log.gz", []string{
		`[2019/08/26 06:22:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	s.writeTmpFile(t, "rpc.tidb.log", []string{
		`[2019/08/26 06:22:18.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:19.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:20.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:21.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:22.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	type timeRange struct{ start, end string }
	cases := []struct {
		search        timeRange
		expectFileNum int
		expect        []string
		levels        []pb.LogLevel
		patterns      []string
	}{
		{
			search:        timeRange{"2019/08/26 06:22:14.000 -04:00", "2019/08/26 06:22:16.000 -04:00"},
			expectFileNum: 1,
			levels:        []pb.LogLevel{pb.LogLevel_Info},
			patterns:      []string{".*TiDB.*"},
			expect: []string{
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		{
			search:        timeRange{"2019/08/26 06:22:10.000 -04:00", "2019/08/26 06:22:16.000 -04:00"},
			expectFileNum: 2,
			levels:        []pb.LogLevel{pb.LogLevel_Info},
			patterns:      []string{".*TiDB.*"},
			expect: []string{
				`[2019/08/26 06:22:10.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:11.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:12.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		{
			search:        timeRange{"2019/08/26 06:22:14.000 -04:00", "2019/08/26 06:22:20.000 -04:00"},
			expectFileNum: 2,
			levels:        []pb.LogLevel{pb.LogLevel_Info},
			patterns:      []string{".*TiDB.*"},
			expect: []string{
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:18.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:19.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		{
			search:        timeRange{"2019/08/26 06:22:10.000 -04:00", "2019/08/26 06:22:20.000 -04:00"},
			expectFileNum: 3,
			levels:        []pb.LogLevel{pb.LogLevel_Info},
			patterns:      []string{".*TiDB.*"},
			expect: []string{
				`[2019/08/26 06:22:10.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:11.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:12.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:18.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:19.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		{
			// When file1.endtime < search.start < file2.begintime,
			// it will scan one more file, because we don't now the last item for a compressed file.
			search:        timeRange{"2019/08/26 06:22:13.000 -04:00", "2019/08/26 06:22:20.000 -04:00"},
			expectFileNum: 3,
			levels:        []pb.LogLevel{pb.LogLevel_Info},
			patterns:      []string{".*TiDB.*"},
			expect: []string{
				`[2019/08/26 06:22:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:18.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:19.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
	}

	// Set up a connection to the server.
	conn, err := grpc.Dial(s.address, grpc.WithInsecure())
	require.NoError(t, err)

	for i, cas := range cases {
		beginTime, err := sysutil.ParseTimeStamp(cas.search.start)
		require.NoError(t, err)
		endTime, err := sysutil.ParseTimeStamp(cas.search.end)
		require.NoError(t, err)

		logfile, err := sysutil.ResolveFiles(context.Background(), filepath.Join(s.tmpDir, "rpc.tidb.log"), beginTime, endTime)
		require.NoError(t, err)
		require.Len(t, logfile, cas.expectFileNum)

		req := &pb.SearchLogRequest{
			StartTime: beginTime,
			EndTime:   endTime,
			Levels:    cas.levels,
			Patterns:  cas.patterns,
		}
		client := pb.NewDiagnosticsClient(conn)

		// Contact the server and print out its response.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		stream, err := client.SearchLog(ctx, req)
		require.NoError(t, err)

		resp := &pb.SearchLogResponse{}
		for {
			res, err := stream.Recv()
			if err != nil && err == io.EOF {
				break
			}
			require.NoError(t, err)
			require.NotNil(t, res)
			resp.Messages = append(resp.Messages, res.Messages...)
		}

		var items []*pb.LogMessage
		for _, s := range cas.expect {
			item, err := sysutil.ParseLogItem(s)
			require.NoError(t, err)
			items = append(items, item)
		}
		require.Equal(t, len(items), len(resp.Messages), fmt.Sprintf("search log (index: %d) failed", i))
		require.Equal(t, items, resp.Messages, fmt.Sprintf("search log (index: %d) failed", i))
	}
}

func BenchmarkReadLastLines(b *testing.B) {
	s, clean := createSearchLogSuite(b)
	defer clean()

	// step 1. initial a log file
	s.writeTmpFile(b, "tidb.log", []string{
		`[2019/08/26 06:22:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// step 2. open it as read only mode
	path := filepath.Join(s.tmpDir, "tidb.log")
	file, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	require.NoError(b, err, fmt.Sprintf("open file %s failed", path))
	defer func() {
		require.NoError(b, file.Close())
	}()

	stat, _ := file.Stat()
	filesize := stat.Size()

	// step 3. start to benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = sysutil.ReadLastLines(context.Background(), file, filesize)
	}
}

func BenchmarkReadLastLinesOfHugeLine(b *testing.B) {
	s, clean := createSearchLogSuite(b)
	defer clean()

	// step 1. initial a huge line log file
	hugeLine := make([]byte, 1024*1024*10)
	for i := range hugeLine {
		hugeLine[i] = 'a' + byte(i%26)
	}
	s.writeTmpFile(b, "tidb.log", []string{string(hugeLine)})

	// step 2. open it as read only mode
	path := filepath.Join(s.tmpDir, "tidb.log")
	file, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	require.NoError(b, err, fmt.Sprintf("open file %s failed", path))
	defer func() {
		require.NoError(b, file.Close())
	}()

	stat, _ := file.Stat()
	filesize := stat.Size()

	// step 3. start to benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = sysutil.ReadLastLines(context.Background(), file, filesize)
	}
}

// run benchmark by `go test -check.b`
// result:
// searchLogSuite.BenchmarkReadLastLines      1000000              2008 ns/op
// searchLogSuite.BenchmarkReadLastLines      1000000              2193 ns/op
// result for the old readLastLine method when last line is 76 bytes long:
// searchLogSuite.BenchmarkReadLastLine        10000            124423 ns/op
// searchLogSuite.BenchmarkReadLastLine        10000            126135 ns/op
// result for the old readLastLine method when last line is 76*2 bytes long:
// searchLogSuite.BenchmarkReadLastLine        10000            247836 ns/op
// searchLogSuite.BenchmarkReadLastLine        10000            251958 ns/op
