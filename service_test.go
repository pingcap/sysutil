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
	"log"
	"net"
	"testing"
	"time"

	. "github.com/pingcap/check"
	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
	"github.com/pingcap/sysutil"
	"google.golang.org/grpc"
)

type serviceSuite struct {
	server  *grpc.Server
	address string
}

var _ = Suite(&serviceSuite{})

func TestT(t *testing.T) {
	TestingT(t)
}

func (s *serviceSuite) SetUpSuite(c *C) {
	server := grpc.NewServer()
	pb.RegisterDiagnosticsServer(server, &sysutil.DiagnosticsServer{})

	// Find a available port
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, IsNil, Commentf("cannot find available port"))

	s.server = server
	s.address = fmt.Sprintf(":%d", listener.Addr().(*net.TCPAddr).Port)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}

func (s *serviceSuite) TearDownSuite(c *C) {
	s.server.Stop()
}

func (s *serviceSuite) TestRPCServerInfo(c *C) {
	// Set up a connection to the server.
	conn, err := grpc.Dial(s.address, grpc.WithInsecure())
	c.Assert(err, IsNil)

	defer conn.Close()
	client := pb.NewDiagnosticsClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Test for load info.
	r, err := client.ServerInfo(ctx, &pb.ServerInfoRequest{Tp: pb.ServerInfoType_LoadInfo})
	c.Assert(err, IsNil)
	c.Assert(r, NotNil)
	c.Assert(len(r.Items), Not(Equals), 0)

	// Test for hardware info.
	r, err = client.ServerInfo(ctx, &pb.ServerInfoRequest{Tp: pb.ServerInfoType_HardwareInfo})
	c.Assert(err, IsNil)
	c.Assert(r, NotNil)
	c.Assert(len(r.Items), Not(Equals), 0)

	// Test for system info.
	r, err = client.ServerInfo(ctx, &pb.ServerInfoRequest{Tp: pb.ServerInfoType_SystemInfo})
	c.Assert(err, IsNil)
	c.Assert(r, NotNil)
	c.Assert(len(r.Items), Not(Equals), 0)
}
