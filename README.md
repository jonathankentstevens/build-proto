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
    rpc Login (LoginRequest) returns (LoginResponse) {}
    rpc Register (RegisterRequest) returns (RegisterResponse) {}
}

message LoginRequest {
    string username = 1;
    string password = 2;
}

message LoginResponse {
    string err = 1;
}

message RegisterRequest {
    string first_name = 1;
    string last_name = 2;
    string email = 3;
    string password = 4;
}

message RegisterResponse {
    string err = 1;
}
```

You would cd to your '$GOPATH/src' directory and run:
```
build-proto path/to/user.proto
```

The following would be the two files created in addition to the normal pb stub:

# client/client.go

```go
// Package client serves as the mechanism to connect to the user gRPC service and execute
// any methods against it
package client

import (
	"path/to/user/proto"
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

// Login is this client's implementation of the UserClient interface
func (c *SvcClient) Login(ctx context.Context, req *proto.LoginRequest, opts ...grpc.CallOption) (*proto.LoginResponse, error) {
	return c.service.Login(ctx, req)
}

// Login...
func Login(ctx context.Context, c proto.UserClient) (*proto.LoginResponse, error) {
	res, err := c.Login(ctx, &proto.LoginRequest{})
	if err != nil {
		return nil, err
	}

	return res, nil
}

// Register is this client's implementation of the UserClient interface
func (c *SvcClient) Register(ctx context.Context, req *proto.RegisterRequest, opts ...grpc.CallOption) (*proto.RegisterResponse, error) {
	return c.service.Register(ctx, req)
}

// Register...
func Register(ctx context.Context, c proto.UserClient) (*proto.RegisterResponse, error) {
	res, err := c.Register(ctx, &proto.RegisterRequest{})
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
	"path/to/user/client"
	"path/to/user/proto"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type testClient struct{}

// Login is the custom implementation of the UserClient interface to allow for unit testing the logic of
// the user gRPC service without requiring a connection to it
func (c *testClient) Login(ctx context.Context, req *proto.LoginRequest, opts ...grpc.CallOption) (*proto.LoginResponse, error) {
	return &proto.LoginResponse{}, nil
}

// Register is the custom implementation of the UserClient interface to allow for unit testing the logic of
// the user gRPC service without requiring a connection to it
func (c *testClient) Register(ctx context.Context, req *proto.RegisterRequest, opts ...grpc.CallOption) (*proto.RegisterResponse, error) {
	return &proto.RegisterResponse{}, nil
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

func TestLogin(t *testing.T) {
	c := new(testClient)
	_, err := client.Login(context.Background(), c)
	if err != nil {
		t.Fatalf("expected nil from Login, got error: %v", err)
	}
}
func TestRegister(t *testing.T) {
	c := new(testClient)
	_, err := client.Register(context.Background(), c)
	if err != nil {
		t.Fatalf("expected nil from Register, got error: %v", err)
	}
}
```

# server/main.go

```go
package main

import (
	"path/to/user/proto"
	"log"
	"net"

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

type loginResponse struct {
	res *proto.LoginResponse
	err error
}

type registerResponse struct {
	res *proto.RegisterResponse
	err error
}

func (s *userServer) Login(ctx context.Context, req *proto.LoginRequest) (*proto.LoginResponse, error) {

	c := make(chan *loginResponse)
	go func(req *proto.LoginRequest) {
		resp := new(loginResponse)

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

func (s *userServer) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {

	c := make(chan *registerResponse)
	go func(req *proto.RegisterRequest) {
		resp := new(registerResponse)

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
