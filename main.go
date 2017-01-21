package main

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"utils/cmd"
	"utils/str"
)

func buildPbFile(protoFile, pbFile string) string {
	cb, err := getContents(pbFile)
	if err != nil {
		log.Fatalln("Read file error:", err.Error())
	}
	contents := string(cb)

	inFile, err := os.Open(protoFile)
	if err != nil {
		log.Fatalln("Proto file read error:", err.Error(), `: `, protoFile)
		return ""
	}
	defer inFile.Close()

	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		txt := scanner.Text()
		if strings.Contains(txt, `import "`) {
			replaceTxt := str.Between(txt, `import "`, `.proto";`)
			args := strings.Split(replaceTxt, "/")
			importPkg := strings.Replace(args[len(args)-1], ".proto", "", 1)
			contents = strings.Replace(contents, `import `+importPkg+` "`+importPkg+`"`, `import "services/`+importPkg+`"`, 1)
		}
	}

	return contents
}

func buildServer(pkg string) string {
	return `package main

import (
	"log"
	"net"
	"os"
	"services/` + pkg + `/proto"

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

	// Create a gRPC server with a logging middleware
	server := grpc.NewServer()

	// Register our service implementation with the server
	proto.Register` + str.UppercaseFirst(pkg) + `Server(server, new(` + pkg + `Server))

	log.Println("Serving on", port)
	log.Fatalln(server.Serve(listener))
}

/*
	TO DO:

		- change 'someResponse' & 'SomeResponse' to match the response type
		- change 'SomeRequest' to match the correct request type
		- change 'SomeMethod' to match the method specified in your .proto file
*/

type ` + pkg + `Server struct{}

type someResponse struct {
	res *SomeResponse
	err error
}

// TODO: finish implementing all methods from .proto file
func (s *` + pkg + `Server) SomeMethod(ctx context.Context, req *SomeRequest) (*SomeResponse, error) {
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
`
}

func buildClient(pkg string) string {
	return `package client

import (
	"services/` + pkg + `/proto"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type Client struct {
	service proto.` + str.UppercaseFirst(pkg) + `Client
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

// NewClient connects to the ` + pkg + ` service and returns a client to be used for calling methods
// against the service.
//
// If the client is already initialized, it will not dial out again. It will just return the client.
func NewClient() (*Client, error) {

	if cl.client != nil {
		return cl.client, nil
	}

	timeout := grpc.WithTimeout(time.Second * 2)

	// localhost:8080 needs to change to whatever the location of the service will be
	// defined as in etcd
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
		service: proto.New` + str.UppercaseFirst(pkg) + `Client(g),
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
`
}

func getContents(path string) ([]byte, error) {

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// It's a good but not certain bet that FileInfo will tell us exactly how much to
	// read, so let's try it but be prepared for the answer to be wrong.
	var n int64

	if fi, err := f.Stat(); err == nil {
		// Don't preallocate a huge buffer, just in case.
		if size := fi.Size(); size < 1e9 {
			n = size
		}
	}
	// As initial capacity for readAll, use n + a little extra in case Size is zero,
	// and to avoid another allocation after Read has filled the buffer.  The readAll
	// call will read into its allocated internal buffer cheaply.  If the size was
	// wrong, we'll either waste some space off the end or reallocate as needed, but
	// in the overwhelmingly common case we'll get it just right.
	return readAll(f, n+bytes.MinRead)
}

// readAll reads from r until an error or EOF and returns the data it read
// from the internal buffer allocated with a specified capacity.
func readAll(r io.Reader, capacity int64) (b []byte, err error) {
	buf := bytes.NewBuffer(make([]byte, 0, capacity))
	// If the buffer overflows, we will get bytes.ErrTooLarge.
	// Return that as an error. Any other panic remains.
	defer func() {
		e := recover()
		if e == nil {
			return
		}
		if panicErr, ok := e.(error); ok && panicErr == bytes.ErrTooLarge {
			err = panicErr
		} else {
			panic(e)
		}
	}()
	_, err = buf.ReadFrom(r)
	return buf.Bytes(), err
}

//write will put contents into a new or existing file.
//
//If file exists and overwrite is 'true' it will remove the file and
//recreate it. If file exists and overwrite is 'false', it will append the contents
//to the file
//
//If file does not exist, it will be created
func write(path, contents string, overwrite bool) error {
	var err error
	if exists(path) && overwrite {
		err = os.Remove(path)
	}

	var file *os.File
	if !exists(path) {
		args := strings.Split(path, "/")
		dir := args[0] + "/" + args[1]
		if !exists(dir) {
			err = os.Mkdir(dir, 0777)
			if err != nil {
				return err
			}
		}
		file, err = os.Create(path)
		defer file.Close()
		if err != nil {
			return err
		}
	} else {
		file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0777)
		defer file.Close()
		if err != nil {
			return err
		}
	}

	_, err = file.WriteString(contents)
	if err != nil {
		return err
	}

	return nil
}

//exists determines whether or not a file or directory exists
func exists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}

	return false
}

func main() {
	file := os.Args[1]
	if file == "" {
		log.Fatalln("You must provide a path to the proto file")
	}

	parts := strings.Split(file, "/")
	protoFile := parts[len(parts)-1]
	pbFile := strings.Replace(protoFile, ".proto", ".pb.go", 1)
	pkg := strings.Replace(pbFile, ".pb.go", "", 1)
	parts = parts[:len(parts)-2]
	dir := strings.Join(parts, "/") + "/"
	protoDir := dir + "proto/"
	clientDir := dir + "client/"
	serverDir := dir + "server/"

	_, err := cmd.Exec("protoc --go_out=plugins=grpc:. "+file, true, false)
	if err != nil {
		log.Fatalln("protoc error:", err.Error())
	}

	contents := buildPbFile(file, protoDir+pbFile)
	err = write(protoDir+pbFile, contents, true)
	if err != nil {
		log.Fatalln("pb write file error:", err.Error())
	}

	serverFileContents := buildServer(pkg)
	serverFile := serverDir + "main.go"
	if !exists(serverFile) {
		err = write(serverFile, serverFileContents, true)
		if err != nil {
			log.Fatalln("server write file error:", err.Error())
		}
	}

	_, err = cmd.Exec("go fmt "+serverFile, true, false)
	if err != nil {
		log.Fatalln("go fmt error:", err.Error())
	}

	clientFileContents := buildClient(pkg)
	clientFile := clientDir + "client.go"
	if !exists(clientFile) {
		err = write(clientFile, clientFileContents, true)
		if err != nil {
			log.Fatalln("client write file error:", err.Error())
		}
	}

	_, err = cmd.Exec("go fmt "+clientFile, true, false)
	if err != nil {
		log.Fatalln("Go fmt error:", err.Error())
	}

	println("Success.")
}
