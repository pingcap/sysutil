package diagnostics

import (
	"context"
	"log"
	"net"
	"testing"
	"time"
	"google.golang.org/grpc"

	pb "github.com/pingcap/kvproto/pkg/diagnosticspb"
)

func TestRPCServerLoadInfo(t *testing.T) {
	address := "127.0.0.1:10080"
	setUpService(address)

	// Set up a connection to the server.
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := pb.NewDiagnosticsClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := c.ServerInfo(ctx, &pb.ServerInfoRequest{Tp:pb.ServerInfoType_LoadInfo})
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || len(r.Items) == 0 {
		t.Fatal()
	}
	return
}

func setUpService(addr string){
	go func() {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		s := grpc.NewServer()
		pb.RegisterDiagnosticsServer(s, &DiagnoseServer{})
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}
