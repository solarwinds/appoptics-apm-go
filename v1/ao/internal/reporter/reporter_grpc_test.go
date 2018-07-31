// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"testing"

	pb "github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	testKeyPath  = path.Join(os.Getenv("GOPATH"), "src/github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter")
	testKeyFile  = path.Join(testKeyPath, "localhost.key")
	testCertFile = path.Join(testKeyPath, "localhost.crt")
)

type TestGRPCServer struct {
	t          *testing.T
	grpcServer *grpc.Server
	addr       string
	// The mutex to protect the other fields, mainly the slices below as gRPC needs concurrency-safe
	// Performance is not a concern for a testing reporter, so we are fine with a single mutex for all
	// the fields.
	mutex   sync.Mutex
	events  []*pb.MessageRequest
	metrics []*pb.MessageRequest
	status  []*pb.MessageRequest
	pings   int
}

func StartTestGRPCServer(t *testing.T, addr string) *TestGRPCServer {
	lis, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	// Create the TLS credentials
	creds, err := credentials.NewServerTLSFromFile(testCertFile, testKeyFile)
	require.NoError(t, err, "could not load TLS keys")
	assert.NotNil(t, creds)

	// Create the gRPC server with the credentials
	grpcServer := grpc.NewServer(grpc.Creds(creds))
	assert.NotNil(t, grpcServer)
	testServer := &TestGRPCServer{t: t, grpcServer: grpcServer, addr: addr}
	pb.RegisterTraceCollectorServer(grpcServer, testServer)
	require.NoError(t, err)

	go grpcServer.Serve(lis)
	return testServer
}

func printMessageRequest(req *pb.MessageRequest) {
	bs, _ := json.Marshal(req)
	fmt.Printf("Raw message marshaled to json->%s\n", bs)
	fmt.Println("Events decoded from BSON->")
	for idx, m := range req.Messages {
		fmt.Printf("#%d->", idx)
		utils.PrintBson(m)
	}
}

func printSettingsRequest(req *pb.SettingsRequest) {
	bs, _ := json.Marshal(req)
	fmt.Printf("Raw message marshaled to json->%s\n", bs)
}

func (s *TestGRPCServer) Stop() { s.grpcServer.Stop() }

func (s *TestGRPCServer) PostEvents(ctx context.Context, req *pb.MessageRequest) (*pb.MessageResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	fmt.Println("TestGRPCServer.PostEvents req:")
	printMessageRequest(req)
	s.events = append(s.events, req)
	if strings.HasPrefix(req.ApiKey, "invalid") {
		return &pb.MessageResult{Result: pb.ResultCode_INVALID_API_KEY}, nil
	}
	return &pb.MessageResult{Result: pb.ResultCode_OK}, nil
}

func (s *TestGRPCServer) PostMetrics(ctx context.Context, req *pb.MessageRequest) (*pb.MessageResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	fmt.Println("TestGRPCServer.PostMetrics req:")
	printMessageRequest(req)
	s.metrics = append(s.metrics, req)
	return &pb.MessageResult{Result: pb.ResultCode_OK}, nil
}

func (s *TestGRPCServer) PostStatus(ctx context.Context, req *pb.MessageRequest) (*pb.MessageResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	fmt.Println("TestGRPCServer.PostStatus req:")
	printMessageRequest(req)
	s.status = append(s.status, req)
	return &pb.MessageResult{Result: pb.ResultCode_OK}, nil
}

func (s *TestGRPCServer) GetSettings(ctx context.Context, req *pb.SettingsRequest) (*pb.SettingsResult, error) {
	fmt.Println("TestGRPCServer.GetSettings req:")
	printSettingsRequest(req)
	return &pb.SettingsResult{
		Result: pb.ResultCode_OK,
		Settings: []*pb.OboeSetting{{
			Type: pb.OboeSettingType_DEFAULT_SAMPLE_RATE,
			// Flags:     XXX,
			// Layer:     "", // default, specifically not setting layer/service
			// Timestamp: XXX,
			Value:     1000000,
			Arguments: map[string][]byte{
				//   "BucketCapacity": XXX,
				//   "BucketRate":     XXX,
			},
			Ttl: 120,
		}},
	}, nil
}

func (s *TestGRPCServer) Ping(ctx context.Context, req *pb.PingRequest) (*pb.MessageResult, error) {
	fmt.Printf("TestGRPCServer.Ping with APIKey: %s\n", req.ApiKey)
	s.pings++
	return &pb.MessageResult{Result: pb.ResultCode_OK}, nil
}
