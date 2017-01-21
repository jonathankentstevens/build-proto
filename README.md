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

If you had a user service:
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

type Client struct {
	service proto.UserClient
}

type syncedClient struct {
	sync.Mutex
	client *Client
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
func NewClient() (*Client, error) {

	if cl.client != nil {
		return cl.client, nil
	}

	timeout := grpc.WithTimeout(time.Second * 2)

	// TODO: host/port needs to be updated
	g, err := grpc.Dial("localhost:8080", grpc.WithInsecure(), timeout)
	if err != nil {
		return nil, err
	}

	// get the service client
	cl.Lock()
	if cl.client != nil {
		cl.Unlock()
		return cl.client, nil
	}
	cl.client = &Client{
		service: proto.NewUserClient(g),
	}
	cl.Unlock()

	return cl.client, err
}

// TODO: finish method(s)
func (c *Client) SomeMethod(ctx context.Context, id int64) (*proto.SomeResponse, error) {

	r, err := c.service.SomeMethod(ctx, &proto.SomeRequest{})
	if err != nil {
		return nil, err
	}

	return r, nil
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
	port string = "8080"
)

func main() {
	// Create a listener to accept incoming requests
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		os.Exit(1)
	}

	// Create a gRPC server
	server := grpc.NewServer()

	// Register our service implementation with the server
	proto.RegisterUserServer(server, new(userServer))

	log.Println("Serving on", port)
	log.Fatalln(server.Serve(listener))
}

/*
	TO DO:

		- change 'someResponse' & 'SomeResponse' to match the response type
		- change 'SomeRequest' to match the correct request type
		- change 'SomeMethod' to match the method specified in your .proto file
*/

type userServer struct{}

type someResponse struct {
	res *SomeResponse
	err error
}

// TODO: finish implementing all methods from .proto file
func (s *userServer) SomeMethod(ctx context.Context, req *SomeRequest) (*SomeResponse, error) {
	//thisLogger := logger.New(ctx) //if needed

	c := make(chan *someResponse)
	go func(req *SomeRequest) {
		resp := new(someResponse)

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
