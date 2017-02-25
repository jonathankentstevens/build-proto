package main

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

type implementation struct {
	method   string
	request  string
	response string
}

var implementations []implementation

// updatePbFile updates the import paths
func updatePbFile(protoFile, pbFile string) string {
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
			replaceTxt := between(txt, `import "`, `.proto";`)
			args := strings.Split(replaceTxt, "/")
			importPkg := strings.Replace(args[len(args)-1], ".proto", "", 1)
			contents = strings.Replace(contents, `import `+importPkg+` "`+importPkg+`/proto"`, `import `+importPkg+` "services/`+importPkg+`/proto"`, 1)
		} else if strings.Contains(txt, "rpc") {
			args := strings.Split(strings.TrimSpace(txt), " ")
			imp := implementation{
				method:   args[1],
				request:  between(args[2], "(", ")"),
				response: between(args[4], "(", ")"),
			}
			implementations = append(implementations, imp)
		}
	}

	return contents
}

// buildServer generates server package string
func buildServer(pkg string) string {
	serverFileContents := `package main

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
	proto.Register` + uppercaseFirst(pkg) + `Server(server, new(` + pkg + `Server))

	log.Println("Serving on", port)
	log.Fatalln(server.Serve(listener))
}

type ` + pkg + `Server struct{}
`
	for _, imp := range implementations {
		serverFileContents += `
type ` + lowercaseFirst(imp.method) + `Response struct {
	res *proto.` + imp.response + `
	err error
}
`
	}

	for _, imp := range implementations {
		serverFileContents += `
func (s *` + pkg + `Server) ` + imp.method + `(ctx context.Context, req *proto.` + imp.request + `) (*proto.` + imp.response + `, error) {

	c := make(chan *` + lowercaseFirst(imp.method) + `Response)
	go func(req *proto.` + imp.request + `) {
		resp := new(` + lowercaseFirst(imp.method) + `Response)

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

	return serverFileContents
}

// buildClient generates client package string
func buildClient(pkg string) string {
	clientFileContents := `package client

import (
	"database/sql"
	"services/` + pkg + `/proto"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type SvcClient struct {
	sync.Mutex
	service proto.` + uppercaseFirst(pkg) + `Client
}

var (
	cl   *SvcClient
	once sync.Once
)

func init() {
	cl = new(SvcClient)
}

// NewClient connects to the ` + pkg + ` service and returns a client to be used for calling methods
// against the service.
//
// If the client is already initialized, it will not dial out again. It will just return the client.
func NewClient() (*SvcClient, error) {

	var clientErr error

	once.Do(func() {
		timeout := grpc.WithTimeout(time.Second * 1)

		// localhost:8000 needs to change to whatever the location of the service will be
		g, err := grpc.Dial("localhost:8000", grpc.WithInsecure(), timeout)
		if err != nil {
			clientErr = err
		}

		// get the service client
		if cl != nil {
			clientErr = err
		}

		cl.service = proto.New` + uppercaseFirst(pkg) + `Client(g)
	})

	return cl, clientErr
}
`

	for _, imp := range implementations {
		clientFileContents += `
// ` + imp.method + ` is this client's implementation of the ` + uppercaseFirst(pkg) + `Client interface
func (c *SvcClient) ` + imp.method + `(ctx context.Context, req *proto.` + imp.request + `, opts ...grpc.CallOption) (*proto.` + imp.response + `, error) {
	return c.service.` + imp.method + `(ctx, req)
}

// ` + imp.method + `...
func ` + imp.method + `(ctx context.Context, c proto.` + uppercaseFirst(pkg) + `Client) (*proto.` + imp.response + `, error) {
	res, err := c.` + imp.method + `(ctx, &proto.` + imp.request + `{})
	if err != nil {
		if strings.Contains(err.Error(), "sql: no results in result set") {
			err = sql.ErrNoRows
		}
		return nil, err
	}

	return res, nil
}`
	}

	return clientFileContents
}

func buildTests(pkg string) string {
	testFileContents := `package client_test

import (
	"services/` + pkg + `/client"
	"services/` + pkg + `/proto"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type testClient struct{}
`

	for _, imp := range implementations {
		testFileContents += `
func (c *testClient) ` + imp.method + `(ctx context.Context, req *proto.` + imp.request + `, opts ...grpc.CallOption) (*proto.` + imp.response + `, error) {
	return &proto.` + imp.response + `{}, nil
}
`
	}

	testFileContents += `
func TestNewClient(t *testing.T) {
	c, err := client.NewClient()
	if err != nil {
		t.Fatalf("unable to connect to gRPC service: %s", err)
	}

	if c == nil {
		t.Fatal("client is nil even though no error was thrown")
	}
}
`

	for _, imp := range implementations {
		testFileContents += `
func Test` + imp.method + `(t *testing.T) {
	c := new(testClient)
	_, err := client.` + imp.method + `(context.Background(), c)
	if err != nil {
		t.Fatalf("expected nil from ` + imp.method + `, got error: %v", err)
	}
}`
	}

	return testFileContents
}

//getContents returns a byte array of the file contents passed in
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

//execute is main method to run command. Allows output to show and whether or
//not to return the stdout into a string variable
func execute(command string, showOutput bool, returnOutput bool) (string, error) {
	if showOutput {
		log.Println("Running command: " + command)
	}

	//honor quotes
	parts := getCmdParts(command)
	if returnOutput {
		data, err := exec.Command(parts[0], parts[1:]...).Output()
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	if showOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return "", nil
}

//getCmdParts normalizes command into a string array
func getCmdParts(command string) []string {
	lastQuote := rune(0)
	f := func(c rune) bool {
		switch {
		case c == lastQuote:
			lastQuote = rune(0)
			return false
		case lastQuote != rune(0):
			return false
		case unicode.In(c, unicode.Quotation_Mark):
			lastQuote = c
			return false
		default:
			return unicode.IsSpace(c)
		}
	}

	var parts []string
	preParts := strings.FieldsFunc(command, f)
	for i := range preParts {
		part := preParts[i]
		parts = append(parts, strings.Replace(part, "'", "", -1))
	}

	return parts
}

//between returns string between two specified characters/strings
func between(initial string, beginning string, end string) string {
	return strings.TrimLeft(strings.TrimRight(initial, end), beginning)
}

//Does what it says it does
func uppercaseFirst(str string) string {
	for i, v := range str {
		return string(unicode.ToUpper(v)) + str[i+1:]
	}
	return ""
}

//Does what it says it does
func lowercaseFirst(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
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

	_, err := execute("protoc --go_out=plugins=grpc:. "+file, true, false)
	if err != nil {
		log.Fatalln("protoc error:", err.Error())
	}

	contents := updatePbFile(file, protoDir+pbFile)
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

	_, err = execute("go fmt "+serverFile, true, false)
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

	_, err = execute("go fmt "+clientFile, true, false)
	if err != nil {
		log.Fatalln("Go fmt error:", err.Error())
	}

	testFileContents := buildTests(pkg)
	testFile := clientDir + "client_test.go"
	if !exists(testFile) {
		err = write(testFile, testFileContents, true)
		if err != nil {
			log.Fatalln("client write file error:", err.Error())
		}
	}

	_, err = execute("go fmt "+testFile, true, false)
	if err != nil {
		log.Fatalln("Go fmt error:", err.Error())
	}

	println("Success.")
}
