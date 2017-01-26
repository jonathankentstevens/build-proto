[![License](http://img.shields.io/:license-gpl3-blue.svg)](http://www.gnu.org/licenses/gpl-3.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/jonathankentstevens/build-proto)](https://goreportcard.com/report/github.com/jonathankentstevens/build-proto)
[![GoDoc](https://godoc.org/github.com/jonathankentstevens/build-proto?status.svg)](https://godoc.org/github.com/jonathankentstevens/build-proto)
[![Build Status](https://travis-ci.org/jonathankentstevens/build-proto.svg?branch=master)](https://travis-ci.org/jonathankentstevens/build-proto)

# build-proto

Command line tool to generate client & server implementation with your pb stub for gRPC Edit

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
package client

import (
	"services/user/proto"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type client struct {
	service proto.UserClient
}

type syncedClient struct {
	sync.Mutex
	client *client
}

var (
	cl *syncedClient
)

func init() {
	cl = new(syncedClient)
}

// NewClient connects to the user service and returns a client to be used for calling methods
// against the service.
//
// If the client is already initialized, it will not dial out again. It will just return the client.
func NewClient() (*client, error) {

	if cl.client != nil {
		return cl.client, nil
	}

	timeout := grpc.WithTimeout(time.Second * 2)

	// localhost:8000 needs to change to whatever the location of the service will be
	g, err := grpc.Dial("localhost:8000", grpc.WithInsecure(), timeout)
	if err != nil {
		return nil, err
	}

	// get the service client
	cl.Lock()
	if cl.client != nil {
		cl.Unlock()
		return cl.client, nil
	}
	cl.client = &client{
		service: proto.NewUserClient(g),
	}
	cl.Unlock()

	return cl.client, err
}

// Authenticate is this client's implementation of the UserClient interface
func (c *client) Authenticate(ctx context.Context, req *proto.AuthRequest, opts ...grpc.CallOption) (*proto.AuthResponse, error) {
	return c.service.Authenticate(ctx, req)
}

// TODO: fill in empty strings
// Authenticate...
func Authenticate(c proto.UserClient, ctx context.Context) (string, error) {
	r, err := c.Authenticate(ctx, &proto.AuthRequest{})
	if err != nil {
		return "", err
	}

	return "", nil
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
	_, err := client.Authenticate(c, context.Background())
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
	"os"
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
		os.Exit(1)
	}

	// Create a gRPC server with a logging middleware
	server := grpc.NewServer()

	// Register our service implementation with the server
	proto.RegisterUserServer(server, new(userServer))

	log.Println("Serving on", port)
	log.Fatalln(server.Serve(listener))
}

type userServer struct{}

type authenticateResponse struct {
	res *proto.AuthResponse
	err error
}

func (s *userServer) Authenticate(ctx context.Context, req *proto.AuthRequest) (*proto.AuthResponse, error) {
	//thisLogger := logger.New(ctx) //if needed

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
