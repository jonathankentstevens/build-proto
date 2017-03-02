[![License](http://img.shields.io/:license-gpl3-blue.svg)](http://www.gnu.org/licenses/gpl-3.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/jonathankentstevens/build-proto)](https://goreportcard.com/report/github.com/jonathankentstevens/build-proto)
[![GoDoc](https://godoc.org/github.com/jonathankentstevens/build-proto?status.svg)](https://godoc.org/github.com/jonathankentstevens/build-proto)
[![Build Status](https://travis-ci.org/jonathankentstevens/build-proto.svg?branch=master)](https://travis-ci.org/jonathankentstevens/build-proto)

# build-proto

Command line tool to generate a client & server implementation of a gRPC service to compliment your pb stub

# implementation
    go get github.com/jonathankentstevens/build-proto
	go install github.com/jonathankentstevens/build-proto
	
# usage

If you had a user service defined as:
```
syntax = "proto3";
package proto;

service User {
    rpc Authenticate (AuthRequest) returns (AuthResponse) {
    }
}

message AuthRequest {
    int64 user_id = 1;
}

message AuthResponse {
    bool authenticated = 1;
}
```

You would cd to your 'src/services' directory and run:
```
build-proto user/proto/user.proto
```

The following would be the two files created in addition to the normal pb stub:

# client/client.go

```go
// Package client serves as the mechanism to connect to the user gRPC service and execute
// any methods against it
package client

import (
	"services/user/proto"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// SvcClient holds the UserClient service connection to allow for safe concurrent access
type SvcClient struct {
	sync.Mutex
	service proto.UserClient
}

var (
	cl *SvcClient
)

func init() {
	cl = new(SvcClient)
}

// NewClient connects to the user service and returns a client to be used for calling methods
// against the service.
//
// If the client is already initialized, it will not dial out again. It will just return the client.
func NewClient() (*SvcClient, error) {

	cl.Lock()
	defer cl.Unlock()
	if cl.service != nil {
		return cl, nil
	}

	timeout := grpc.WithTimeout(time.Second * 1)

	// localhost:8000 needs to change to whatever the location of the service will be
	g, err := grpc.Dial("localhost:8000", grpc.WithInsecure(), timeout)
	if err != nil {
		return nil, err
	}

	cl.service = proto.NewUserClient(g)

	return cl, nil
}

// Authenticate is this client's implementation of the UserClient interface
func (c *SvcClient) Authenticate(ctx context.Context, req *proto.AuthRequest, opts ...grpc.CallOption) (*proto.AuthResponse, error) {
	return c.service.Authenticate(ctx, req)
}

// Authenticate...
func Authenticate(ctx context.Context, c proto.UserClient) (*proto.AuthResponse, error) {
	res, err := c.Authenticate(ctx, &proto.AuthRequest{})
	if err != nil {
		return nil, err
	}

	return res, nil
}
```

# client/client_test.go
```go
package client_test

import (
	"services/user/client"
	"services/user/proto"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type testClient struct{}

// Authenticate is the custom implementation of the UserClient interface to allow for unit testing the logic of
// the user gRPC service without requiring a connection to it
func (c *testClient) Authenticate(ctx context.Context, req *proto.AuthRequest, opts ...grpc.CallOption) (*proto.AuthResponse, error) {
	return &proto.AuthResponse{}, nil
}

func TestNewClient(t *testing.T) {
	c, err := client.NewClient()
	if err != nil {
		t.Fatalf("unable to connect to gRPC service: %s", err)
	}

	if c == nil {
		t.Fatal("client is nil even though no error was thrown")
	}
}

func TestAuthenticate(t *testing.T) {
	c := new(testClient)
	_, err := client.Authenticate(context.Background(), c)
	if err != nil {
		t.Fatalf("expected nil from Authenticate, got error: %v", err)
	}
}
```

# server/main.go

```go
package main

import (
	"log"
	"net"
	"services/user/proto"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	port string = "8000"
)

func main() {
	// Create a listener to accept incoming requests
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		// handle error
	}

	// Create a gRPC server
	server := grpc.NewServer()

	// Register our service implementation with the server
	proto.RegisterUserServer(server, new(userServer))

	log.Println("Serving on", port)
	err = server.Serve(listener)
	if err != nil {
		// handle error
	}
}

type userServer struct{}

type authenticateResponse struct {
	res *proto.AuthResponse
	err error
}

func (s *userServer) Authenticate(ctx context.Context, req *proto.AuthRequest) (*proto.AuthResponse, error) {

	c := make(chan *authenticateResponse)
	go func(req *proto.AuthRequest) {
		resp := new(authenticateResponse)

		//do your stuff here to build the resp object

		c <- resp
	}(req)

	for {
		select {
		case <-ctx.Done():
			return nil, grpc.Errorf(codes.Canceled, "some error message")
		case result := <-c:
			return result.res, result.err
		}
	}
}
```
