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

package sysutil_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/pingcap/check"
	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/pingcap/sysutil"
	"google.golang.org/grpc"
)

type searchLogSuite struct {
	server  *grpc.Server
	address string
	tmpDir  string
}

var _ = Suite(&searchLogSuite{})

func (s *searchLogSuite) SetUpSuite(c *C) {
	tmpDir, err := ioutil.TempDir("", "sysutil")
	c.Assert(err, IsNil)

	server := grpc.NewServer()
	pb.RegisterDiagnosticsServer(server, sysutil.NewDiagnosticsServer(filepath.Join(tmpDir, "rpc.tidb.log")))

	// Find a available port
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, IsNil, Commentf("cannot find available port"))

	s.tmpDir = tmpDir
	s.server = server
	s.address = fmt.Sprintf(":%d", listener.Addr().(*net.TCPAddr).Port)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}

func (s *searchLogSuite) TearDownSuite(c *C) {
	s.server.Stop()
	c.Assert(os.RemoveAll(s.tmpDir), IsNil, Commentf("remote tmpDir %v failed", s.tmpDir))
}

func (s *searchLogSuite) writeTmpFile(c *C, filename string, lines []string) {
	err := ioutil.WriteFile(filepath.Join(s.tmpDir, filename), []byte(strings.Join(lines, "\n")), os.ModePerm)
	c.Assert(err, IsNil, Commentf("write tmp file %s failed", filename))
}

func (s *searchLogSuite) TestResoveFiles(c *C) {
	s.writeTmpFile(c, "tidb.log", []string{
		`[2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:16.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:17.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// single line file
	s.writeTmpFile(c, "tidb.log.1", []string{
		`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// two lines file
	s.writeTmpFile(c, "tidb.log.2", []string{
		`[2019/08/26 06:21:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// empty file
	s.writeTmpFile(c, "tidb.log.3", []string{``})
	s.writeTmpFile(c, "tidb.log.4", []string{
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
		c.Assert(err, IsNil)
		endTime, err := sysutil.ParseTimeStamp(cas.search.end)
		c.Assert(err, IsNil)
		logFiles, err := sysutil.ResolveFiles(filepath.Join(s.tmpDir, "tidb.log"), beginTime, endTime)
		c.Assert(err, IsNil)
		c.Assert(len(logFiles), Equals, len(cas.expect), Commentf("search range (index: %d): %+v", i, cas.search))

		for j, exp := range cas.expect {
			beginTime, err := sysutil.ParseTimeStamp(exp.start)
			c.Assert(err, IsNil)
			endTime, err := sysutil.ParseTimeStamp(exp.end)
			c.Assert(err, IsNil)
			c.Assert(beginTime, Equals, logFiles[j].BeginTime(), Commentf("case index: %d, expect index: %v", i, j))
			c.Assert(endTime, Equals, logFiles[j].EndTime(), Commentf("case index: %d, expect index: %v", i, j))
		}
	}
}

func (s *searchLogSuite) TestLogIterator(c *C) {
	s.writeTmpFile(c, "rpc.tidb.log", []string{
		`[2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// single line file
	s.writeTmpFile(c, "rpc.tidb.log.1", []string{
		`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// two lines file
	s.writeTmpFile(c, "rpc.tidb.log.2", []string{
		`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
	})

	// empty file
	s.writeTmpFile(c, "rpc.tidb.log.3", []string{``})
	s.writeTmpFile(c, "rpc.tidb.log.4", []string{
		`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
		`[2019/08/26 06:22:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
	})
	s.writeTmpFile(c, "rpc.tidb.log.5", []string{
		`[2019/08/26 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test to TiDB."]`,
	})

	type timeRange struct{ start, end string }

	// filter by time range
	cases := []struct {
		search  timeRange
		expect  []string
		levels  []pb.LogLevel
		pattern string
	}{
		// 0
		{
			search: timeRange{"2019/08/26 06:19:13.011 -04:00", "2019/08/26 06:22:17.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:13.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
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
				`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test to TiDB."]`,
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
				`[2019/08/26 06:22:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test to TiDB."]`,
			},
		},
		// 4
		{
			search: timeRange{"2019/08/26 06:19:15.011 -04:00", "2019/08/26 06:22:14.011 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:17.011 -04:00] [TRACE] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:20:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:21:15.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:22:14.011 -04:00] [INFO] [printer.go:41] ["Welcome to TiDB."]`,
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
			},
		},
		// 7
		{
			search: timeRange{"2019/08/26 06:19:14.011 -04:00", "2019/08/26 06:19:16.012 -04:00"},
			expect: []string{
				`[2019/08/26 06:19:14.011 -04:00] [WARN] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:15.011 -04:00] [ERROR] [printer.go:41] ["Welcome to TiDB."]`,
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
			},
		},
		// 8
		{
			search: timeRange{"2019/08/26 06:19:15.011 -04:00", "2019/08/26 06:22:14.011 -04:00"},
			levels: []pb.LogLevel{pb.LogLevel_Debug},
			expect: []string{
				`[2019/08/26 06:19:16.011 -04:00] [DEBUG] [printer.go:41] ["Welcome to TiDB."]`,
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
			search:  timeRange{"2019/08/26 06:19:14.011 -04:00", "2019/08/26 06:23:17.011 -04:00"},
			levels:  []pb.LogLevel{pb.LogLevel_Info},
			pattern: "partern",
			expect: []string{
				`[2019/08/26 06:23:14.011 -04:00] [INFO] [printer.go:41] ["partern test to TiDB."]`,
			},
		},
	}

	// Set up a connection to the server.
	conn, err := grpc.Dial(s.address, grpc.WithInsecure())
	c.Assert(err, IsNil)

	defer conn.Close()

	for i, cas := range cases {
		beginTime, err := sysutil.ParseTimeStamp(cas.search.start)
		c.Assert(err, IsNil)
		endTime, err := sysutil.ParseTimeStamp(cas.search.end)
		c.Assert(err, IsNil)
		req := &pb.SearchLogRequest{
			StartTime: beginTime,
			EndTime:   endTime,
			Levels:    cas.levels,
			Pattern:   cas.pattern,
		}
		client := pb.NewDiagnosticsClient(conn)

		// Contact the server and print out its response.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		stream, err := client.SearchLog(ctx, req)
		c.Assert(err, IsNil)

		resp := &pb.SearchLogResponse{}
		for {
			res, err := stream.Recv()
			if err != nil && err == io.EOF {
				break
			}
			resp.Messages = append(resp.Messages, res.Messages...)
		}

		var items []*pb.LogMessage
		for _, s := range cas.expect {
			item, err := sysutil.ParseLogItem(s)
			c.Assert(err, IsNil)
			items = append(items, item)
		}
		c.Assert(len(resp.Messages), Equals, len(items), Commentf("search log (index: %d) failed", i))
		c.Assert(resp.Messages, DeepEquals, items, Commentf("search log (index: %d) failed", i))
	}
}

func (s *searchLogSuite) TestParseLogLevel(c *C) {
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
		{"invalid", pb.LogLevel_Info},
	}

	for _, cas := range cases {
		c.Assert(sysutil.ParseLogLevel(cas.s), Equals, cas.l, Commentf("parse %v, expected: %v, got: %v", cas.s, cas.l))
	}
}

func (s *searchLogSuite) TestParseLogItem(c *C) {
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
		c.Assert(err, IsNil)
		c.Assert(item.Level, Equals, cas.level)
		time, err := sysutil.ParseTimeStamp(cas.time)
		c.Assert(err, IsNil)
		c.Assert(item.Time, Equals, time)
		c.Assert(item.Message, Equals, cas.message)
	}
}
