/*

This package specifies the application's interface to the distributed
file system (DFS) system to be used in assignment 2 of UBC CS 416
2017W2.

*/

package dfslib

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/rpc"
	"os"
	"regexp"
)

// Represents a type of file access.
type FileMode int

// A Chunk is the unit of reading/writing in DFS.
type Chunk [32]byte

const (
	// Read mode.
	READ FileMode = iota

	// Read/Write mode.
	WRITE

	// Disconnected read mode.
	DREAD
)

const (
	DFS_INIT_PORT = "8090"
)

// Represents a file in the DFS system.
type DFSFile interface {
	// Reads chunk number chunkNum into storage pointed to by
	// chunk. Returns a non-nil error if the read was unsuccessful.
	//
	// Can return the following errors:
	// - DisconnectedError (in READ,WRITE modes)
	// - ChunkUnavailable (in READ,WRITE modes)
	Read(chunkNum uint8, chunk *Chunk) (err error)

	// Writes chunk number chunkNum from storage pointed to by
	// chunk. Returns a non-nil error if the write was unsuccessful.
	//
	// Can return the following errors:
	// - BadFileModeError (in READ,DREAD modes)
	// - DisconnectedError (in WRITE mode)
	Write(chunkNum uint8, chunk *Chunk) (err error)

	// Closes the file/cleans up. Can return the following errors:
	// - DisconnectedError
	Close() error
}

// Represents a connection to the DFS system.
type DFS interface {
	// Check if a file with filename fname exists locally (i.e.,
	// available for DREAD reads).
	//
	// Can return the following errors:
	// - BadFilename (if filename contains non alpha-numeric chars or is not 1-16 chars long)
	LocalFileExists(fname string) (exists bool, err error)

	// Check if a file with filename fname exists globally.
	//
	// Can return the following errors:
	// - BadFilename (if filename contains non alpha-numeric chars or is not 1-16 chars long)
	// - DisconnectedError
	GlobalFileExists(fname string) (exists bool, err error)

	// Opens a filename with name fname using mode. Creates the file
	// in READ/WRITE modes if it does not exist. Returns a handle to
	// the file through which other operations on this file can be
	// made.
	//
	// Can return the following errors:
	// - DisconnectedError (in READ,WRITE modes)
	// - FileDoesNotExistError (in DREAD mode)
	// - BadFilename (if filename contains non alpha-numeric chars or is not 1-16 chars long)
	Open(fname string, mode FileMode) (f DFSFile, err error)

	// Disconnects from the server. Can return the following errors:
	// - DisconnectedError
	UMountDFS() error
}

// The constructor for a new DFS object instance. Takes the server's
// IP:port address string as parameter, the localIP to use to
// establish the connection to the server, and a localPath path on the
// local filesystem where the client has allocated storage (and
// possibly existing state) for this DFS.
//
// The returned dfs instance is singleton: an application is expected
// to interact with just one dfs at a time.
//
// This call should succeed regardless of whether the server is
// reachable. Otherwise, applications cannot access (local) files
// while disconnected.
//
// Can return the following errors:
// - LocalPathError
// - Networking errors related to localIP or serverAddr

type FileOpArgs struct {
	Filename string
	Mode     FileMode
	ChunkNum uint8
	Chunk    Chunk
	Success  bool
	ErrMsg   string
}

type ChunkOpArgs struct {
	Filename string
	ChunkNum uint8
	Chunk    Chunk
}

var dfsPath string
var initListener net.Listener

var Cache = map[string]*DFSRemoteFile{}

func MountDFS(serverAddr string, localIP string, localPath string) (dfs DFS, err error) {
	// set up local rpc receive
	dfsPath = localPath
	rpcInit := RPCInit{}
	initRpcServer := rpc.NewServer()
	initRpcServer.Register(&rpcInit)
	localTcpAddr, err := net.ResolveTCPAddr("tcp", localIP+":"+DFS_INIT_PORT)
	if err != nil {
		// fmt.Println(err)
		return nil, err
	}
	initListener, err = net.ListenTCP("tcp", localTcpAddr)
	if err != nil {
		// fmt.Println(err)
		return nil, err
	}
	go initRpcServer.Accept(initListener)

	client := DFSClient{}
	client.outgoingClient, err = rpc.Dial("tcp", serverAddr)

	_, err = ioutil.ReadDir(dfsPath)
	if err != nil {
		// fmt.Println(err)
		return nil, LocalPathError(dfsPath)
	}
	return &client, nil
}

type RPCInit struct {
}

func (r *RPCInit) Setup(ip string, port *int) error {
	// fmt.Println("Starting RPC Setup")
	tcpAddr, err := net.ResolveTCPAddr("tcp", ip+":0")
	if err != nil {
		// fmt.Println(err)
		return err
	}
	// fmt.Printf("TCP Addr: IP %s, Port %d\n", tcpAddr.IP.String(), tcpAddr.Port)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		// fmt.Println(err)
		return err
	}
	newAddr, err := net.ResolveTCPAddr("tcp", listener.Addr().String())
	// fmt.Printf("New Addr: IP %s, Port %d\n", newAddr.IP.String(), newAddr.Port)
	server := DFSServer{}
	rpcServer := rpc.NewServer()
	rpcServer.Register(&server)
	go rpcServer.Accept(listener)
	*port = newAddr.Port
	// fmt.Printf("Returning %d as port\n", *port)
	return nil
}

type DFSServer struct {
	Id         string
	ServerAddr string
}

func (s *DFSServer) Identify(ipPort string, result *string) error {
	// fmt.Printf("Identifying to server at %s....\n", ipPort)
	s.ServerAddr = ipPort
	// if Id file exists
	contents, err := ioutil.ReadFile(dfsPath + "/.dfsid")
	if err == nil {
		s.Id = string(contents)
	}
	*result = s.Id
	// send the contents
	return nil
}

func (s *DFSServer) SetIdentity(id string, result *string) error {
	// fmt.Printf("Setting identity to %s....\n", id)
	// write to id file
	ioutil.WriteFile(dfsPath+"/.dfsid", []byte(id), os.FileMode(0777))
	s.Id = id
	return nil
}

func (s *DFSServer) CloseInit(port int, result *string) error {
	initListener.Close()
	return nil
}

func (s *DFSServer) FetchChunk(args ChunkOpArgs, response *ChunkOpArgs) error {
	// sends chunk to server
	file := Cache[args.Filename]
	response = &args
	response.Chunk = file.Chunks[args.ChunkNum]
	return nil
}

func (s *DFSServer) SetChunk(args ChunkOpArgs, response *ChunkOpArgs) error {
	// writes chunk to cache
	file := Cache[args.Filename]
	file.Chunks[args.ChunkNum] = args.Chunk
	response = &args
	return nil
}

type DFSClient struct {
	localPath      string
	outgoingClient *rpc.Client
	incomingClient *DFSServer
}

func (c *DFSClient) Open(fname string, mode FileMode) (f DFSFile, err error) {
	// fmt.Println("=============")
	// fmt.Println("OPEN")
	// fmt.Println("=============")
	// fmt.Printf("Open has been called ")
	// Validate file name
	var ValidFileName = regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString
	if !ValidFileName(fname) || len(fname) > 16 {
		return nil, BadFilenameError(fname)
	}
	// fmt.Printf("with valid fname %s ", fname)
	// check if exists in cache
	cacheExists := Cache[fname] != nil
	if !cacheExists {
		// fmt.Printf("that doesn't exist in the cache %v\n", Cache)
		// check if exists globally
		goballyExists, err := c.GlobalFileExists(fname)
		if err != nil {
			err = fmt.Errorf("Error checking if file exists globally: %v", err)
			return nil, err
		}
		// if new file
		if !goballyExists {
			// create locally
			// fmt.Printf("Creating file %s in %s\n", fname, dfsPath)
			osFilePtr, err := os.Create(dfsPath + "/" + fname + ".dfs")
			if err != nil {
				err = fmt.Errorf("Error creating new file: %v", err)
				return nil, err
			}
			// create DFSRemoteFile Representation
			dfsFile := DFSRemoteFile{
				filename:    fname,
				Client:      c,
				diskFilePtr: osFilePtr,
				Chunks:      make([]Chunk, 256),
				mode:        mode,
			}
			// Add to local cache
			Cache[fname] = &dfsFile
		} else {
			// the file is available elsewhere
			// create the cached copy
			dfsFile := DFSRemoteFile{
				filename: fname,
				Client:   c,
				Chunks:   make([]Chunk, 256),
				mode:     mode,
			}
			Cache[fname] = &dfsFile
			// fetch from server
			args := FileOpArgs{
				Filename: fname,
			}
			response := FileOpArgs{}
			err = c.outgoingClient.Call("DFSClient.DownloadFile", args, &response)
		}
	}
	// fmt.Printf("%s does exist in cache %v as %v\n", fname, Cache, Cache[fname].fileDetails())
	// by this point the file exists in the cache

	// fmt.Println("continuing....")
	// fmt.Printf("mode: %s", fileModeAsString(mode))
	// call open on the server
	args := FileOpArgs{
		Filename: fname,
		Mode:     mode,
	}
	response := &args
	// fmt.Printf("opening with response   %v....\n", response)
	err = c.outgoingClient.Call("DFSClient.Open", args, &response)
	if err != nil {
		err = fmt.Errorf("Error opening file: %v", err)
		return nil, err
	}
	// fmt.Printf("past open with response %v....\n", response)
	// fmt.Printf("allowed? %v....\n", response.Success)
	if !response.Success {
		err = fmt.Errorf("Error open mode denied: %v", err)
		return nil, OpenWriteConflictError("File already open for writing")
	}
	// fmt.Println("past allowed....")
	// fmt.Printf("Cache: %v\n", Cache)
	// fmt.Printf("Cache[fname]: %v\n", Cache[fname].fileDetails())
	// fmt.Println("=============")
	// fmt.Println("END OPEN")
	// fmt.Println("=============")
	return Cache[fname], nil
}

// DREAD use case
func (c *DFSClient) LocalFileExists(fname string) (exists bool, err error) {
	// TODO
	// check cache
	fileList, err := ioutil.ReadDir(c.localPath)
	for _, file := range fileList {
		if file.Name() == fname {
			return true, nil
		}
	}
	return false, nil
}

func (c *DFSClient) GlobalFileExists(fname string) (exists bool, err error) {
	exists, err = c.LocalFileExists(fname)
	if err != nil {
		err = fmt.Errorf("Error checking if file exists locally: %v", err)
		return false, err
	}
	if exists {
		return true, nil
	}
	args := FileOpArgs{
		Filename: fname,
		Success:  false,
	}
	response := FileOpArgs{}
	err = c.outgoingClient.Call("DFSClient.FileExists", args, &response)
	if err != nil {
		err = fmt.Errorf("Error checking if file exists globally: %v", err)
		return false, err
	}
	if response.Success {
		return true, nil
	}
	return false, nil
}

func (c *DFSClient) UMountDFS() (err error) {
	// TODO Close any open local files
	args := FileOpArgs{
		Success: false,
	}
	response := FileOpArgs{}
	err = c.outgoingClient.Call("DFSClient.UMount", args, &response)
	if err != nil {
		err = fmt.Errorf("Error checking if file exists globally: %v", err)
		return err
	}
	return nil
}

type DFSRemoteFile struct {
	filename string
	mode     FileMode
	Chunks   []Chunk
	Client   *DFSClient
	// only not nil when local file
	diskFilePtr *os.File
}

func (f *DFSRemoteFile) Read(chunkNum uint8, chunk *Chunk) (err error) {
	// fmt.Println("=============")
	// fmt.Println("READ")
	// fmt.Println("=============")
	// fmt.Printf("attempting read on file: %v\n", f.fileDetails())
	args := FileOpArgs{
		Filename: f.filename,
		Mode:     f.mode,
		ChunkNum: chunkNum,
	}
	response := &args
	// err = f.Client.outgoingClient.Call("DFSClient.Open", args, &response)
	err = f.Client.outgoingClient.Call("DFSClient.Read", args, response)
	if err != nil {
		err = fmt.Errorf("Error reading file %s: %v", f.filename, err)
		return err
	}
	f.Chunks[chunkNum] = *chunk
	chunk = &response.Chunk
	// fmt.Printf("file after read: %v\n", f.fileDetails())

	// fmt.Println("=============")
	// fmt.Println("END READ")
	// fmt.Println("=============")
	return nil
}

func (f *DFSRemoteFile) Write(chunkNum uint8, chunk *Chunk) (err error) {
	// fmt.Println("=============")
	// fmt.Println("WRITE")
	// fmt.Println("=============")
	// fmt.Printf("Chunk: %v", chunk)
	// fmt.Printf("attempting write on file: %v\n", f.fileDetails())
	if f.mode != WRITE {
		return fmt.Errorf("Not in write mode!")
	}
	args := FileOpArgs{
		Filename: f.filename,
		Mode:     f.mode,
		ChunkNum: chunkNum,
	}
	response := FileOpArgs{}
	err = f.Client.outgoingClient.Call("DFSClient.Write", args, &response)
	if err != nil {
		err = fmt.Errorf("Error writing file %s: %v", f.filename, err)
		return err
	}
	// fmt.Printf("file after write: %v\n", f.fileDetails())

	// fmt.Println("=============")
	// fmt.Println("END WRITE")
	// fmt.Println("=============")
	return nil
}

func (f *DFSRemoteFile) Close() (err error) {
	// close the remote version
	args := FileOpArgs{
		Filename: f.filename,
		Mode:     f.mode,
	}
	response := FileOpArgs{}
	err = f.Client.outgoingClient.Call("DFSClient.Close", args, &response)
	if err != nil {
		err = fmt.Errorf("Error closing file %s: %v", f.filename, err)
		return err
	}
	// if local file
	if f.diskFilePtr != nil {
		f.diskFilePtr.Close()
		f.diskFilePtr = nil
	}
	return nil
}

func (f *DFSRemoteFile) fileDetails() string {
	str := fmt.Sprintf("\nFile at %v:\n", &f)
	str += fmt.Sprintf("    Filename: %s\n", f.filename)
	str += fmt.Sprintf("    Mode: %v\n", fileModeAsString(f.mode))
	str += fmt.Sprintf("    Chunks as String: '%s'\n", chunksAsString(f.Chunks))
	str += fmt.Sprintf("    Client: %v\n", f.Client)
	str += fmt.Sprintf("    Local File?: %v\n", f.diskFilePtr != nil)
	return str
}

func fileModeAsString(mode FileMode) (modeStr string) {
	switch mode {
	case READ:
		modeStr = "READ"
	case WRITE:
		modeStr = "WRITE"
	case DREAD:
		modeStr = "DREAD"
	default:
		modeStr = "UNKNOWN"
	}
	return modeStr
}

func chunksAsString(chunks []Chunk) string {
	str := ""
	for _, chunk := range chunks {
		for _, b := range chunk {
			str += string(b)
		}
	}
	return str
}
