package dfs_server

import (
	"fmt"
	"net"
	"net/rpc"
	"serverlib/fingerprinter"
)

var Files = map[string]*DFSFile{}

var Clients = map[string]*DFSClient{}

// Represents a type of file access.
type FileMode int

const (
	// Read mode.
	READ FileMode = iota

	// Read/Write mode.
	WRITE
)

type State int

const (
	DIRTY State = iota
	CLEAN
)

type Chunk [32]byte

type VersionedChunk struct {
	// list of ids in increasing order of version
	WriteVersionHistory []string
	OpenVersionHistory  map[int]*[]string
}

type DFSFile struct {
	Locked bool
	// TODO: Remove?
	CreatorId       string
	VersionedChunks []*VersionedChunk
}

func (f *DFSFile) printableFileString() string {
	vCs := "["
	for _, chunk := range f.VersionedChunks {
		if chunk != nil {
			oVH := "{"
			for version, history := range chunk.OpenVersionHistory {
				oVH += fmt.Sprintf("%d: %v, ", version, *history)
			}
			oVH += "}"
			vCs += fmt.Sprintf("%v | %s, ", chunk.WriteVersionHistory, oVH)
		} else {
			vCs += "nil, "
		}
	}
	vCs += "]"

	str := fmt.Sprintf("DFSFile at %v:\n", &f)
	str += fmt.Sprintf("    Locked?: %v\n", f.Locked)
	str += fmt.Sprintf("    CreatorID: %s\n", f.CreatorId)
	str += fmt.Sprintf("    VersionedChunks: %v\n", vCs)
	return str
}

// ===============
// RPC Arg Structs
// ===============

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

func (f *FileOpArgs) printableDetails() string {
	cStr := ""
	for _, b := range f.Chunk {
		cStr += string(b)
	}
	str := fmt.Sprintf("FileOpArgs at %v:\n", &f)
	str += fmt.Sprintf("    Filename: %s\n", f.Filename)
	str += fmt.Sprintf("    Mode: %v\n", fileModeAsString(f.Mode))
	str += fmt.Sprintf("    ChunkNum: %d\n", f.ChunkNum)
	str += fmt.Sprintf("    Chunk as String: '%s'\n", cStr)
	str += fmt.Sprintf("    Success: %v\n", f.Success)
	str += fmt.Sprintf("    ErrMsg: %v\n", f.ErrMsg)
	return str
}

// ===============
// BEGIN DFSClient
// ===============

type DFSClient struct {
	id              string
	outgoingRPCConn *rpc.Client
	connected       bool
	state           State
}

// ================
// RPC Enabled Calls
// ================

func (c *DFSClient) Open(args FileOpArgs, response *FileOpArgs) (err error) {
	// fmt.Println("=============")
	// fmt.Printf("OPEN from %s\n", c.id)
	// fmt.Println("=============")
	// fmt.Printf("Args: \n%s\n", args.printableDetails())
	// check if that file has been opened before
	if Files[args.Filename] == nil {
		newFile := DFSFile{
			Locked:          false,
			CreatorId:       c.id,
			VersionedChunks: make([]*VersionedChunk, 256),
		}
		Files[args.Filename] = &newFile
	} else {
		// check a copy is available
		// for each chunk,
		for chunkNum, chunk := range Files[args.Filename].VersionedChunks {
			// if non trivial
			if chunk != nil {
				// ask for bestClient list
				clients, err := findBestClients(args.Filename, uint8(chunkNum))
				if err != nil {
					// fmt.Println(err)
					response.Success = false
					response.ErrMsg = fmt.Sprintf("Error %s while fetching %d for %s!", err, chunkNum, args.Filename)
					return nil
				}
				// if empty fail as unavailable
				if len(clients) == 0 {
					response.Success = false
					response.ErrMsg = fmt.Sprintf("Chunk %d for %s is unavailable!", chunkNum, args.Filename)
					return nil
				}
			}
		}
	}
	// fmt.Printf("File %v\n", Files[args.Filename].printableFileString())
	// file exists and is available
	// fmt.Printf("Files: %v\n", Files)
	if args.Mode == READ {
		// for each chunk that needs it, send updated chunk
		err = c.updateFile(args.Filename)
		// then allow
		response.Success = true
	} else {
		// must be write
		// if the file is not locked
		file := Files[args.Filename]
		if !file.Locked {
			// lock it
			file.Locked = true
			// fetch the most recent version and update the local file
			c.updateFile(args.Filename)
			// allow
			response.Success = true
		} else {
			// disallow
			response.Success = false
			response.ErrMsg = "Unable to open for writing; file locked"
		}
	}
	// fmt.Printf("File %v\n", Files[args.Filename].printableFileString())
	// fmt.Printf("Response: \n%s\n", response.printableDetails())
	// fmt.Println("=============")
	// fmt.Printf("END OPEN from %s\n", c.id)
	// fmt.Println("=============")
	return nil
}

// Assume already allowed
func (c *DFSClient) Read(args FileOpArgs, response *FileOpArgs) (err error) {
	// fmt.Println("=============")
	// fmt.Printf("READ from %s\n", c.id)
	// fmt.Println("=============")
	// fmt.Printf("Args: \n%s\n", args.printableDetails())
	// fmt.Printf("File %v\n", Files[args.Filename].printableFileString())
	// find the someone with the most recent chunk, preferably writer
	// otherwise last opener.
	clients, err := findBestClients(args.Filename, args.ChunkNum)
	if err != nil || len(clients) == 0 {
		response.Success = false
		response.ErrMsg = fmt.Sprintf("Unable to find available client for %s: %v", args.Filename, err)
		return nil
	}
	chunk := Chunk{}
	// fetches chunk from client
	for _, client := range clients {
		chunk, err = client.fetchChunk(args.Filename, args.ChunkNum)
		if err == nil {
			// we got the chunk
			break
		}
	}
	if err != nil {
		response.Success = false
		response.ErrMsg = fmt.Sprintf("Unable to fetch chunk for %s: %v", args.Filename, err)
		return nil
	}
	// we have the chunk
	// add to updated list
	err = addClientToOpenVersionHistory(c.id, args.Filename, args.ChunkNum)
	if err != nil {
		response.Success = false
		response.ErrMsg = fmt.Sprintf("Unable to augment history for %s: %v", args.Filename, err)
		return nil
	}

	response.Chunk = chunk
	response.Success = true
	// fmt.Printf("File %v\n", Files[args.Filename].printableFileString())
	// fmt.Printf("Response: \n%s\n", response.printableDetails())
	// fmt.Println("=============")
	// fmt.Printf("END READ from %s\n", c.id)
	// fmt.Println("=============")
	return nil
}

// Assume already allowed
func (c *DFSClient) Write(args FileOpArgs, response *FileOpArgs) error {
	// fmt.Println("=============")
	// fmt.Printf("WRITE from %s\n", c.id)
	// fmt.Println("=============")
	// fmt.Printf("Args: \n%s\n", args.printableDetails())
	// fmt.Printf("File %v\n", Files[args.Filename].printableFileString())

	response = &args
	file := Files[args.Filename]
	// check if chunk is trivial (never written)
	if file.VersionedChunks[args.ChunkNum] == nil {
		// create the versionedChunk as empty
		vChunk := VersionedChunk{
			WriteVersionHistory: []string{},
			OpenVersionHistory:  map[int]*[]string{},
		}
		file.VersionedChunks[args.ChunkNum] = &vChunk
	}
	// append to the version with c.id for chunkNum
	versions := file.VersionedChunks[args.ChunkNum].WriteVersionHistory
	appendedVersion := append(versions, c.id)
	file.VersionedChunks[args.ChunkNum].WriteVersionHistory = appendedVersion
	// fmt.Printf("File %v\n", Files[args.Filename].printableFileString())
	// fmt.Printf("Response: \n%s\n", response.printableDetails())
	// fmt.Println("=============")
	// fmt.Printf("END WRITE from %s\n", c.id)
	// fmt.Println("=============")
	return nil
}

func (c *DFSClient) Close(args FileOpArgs, response *FileOpArgs) error {
	// fmt.Println("=============")
	// fmt.Printf("CLOSE from %s\n", c.id)
	// fmt.Println("=============")
	// fmt.Printf("Args: \n%s\n", args.printableDetails())
	if args.Mode == WRITE {
		Files[args.Filename].Locked = false
	}
	// fmt.Printf("Response: \n%s\n", response.printableDetails())
	// fmt.Println("=============")
	// fmt.Printf("END CLOSE from %s\n", c.id)
	// fmt.Println("=============")
	return nil
}

func (c *DFSClient) FileExists(args FileOpArgs, response *FileOpArgs) error {
	// fmt.Println("=============")
	// fmt.Printf("FILE_EXISTS from %s\n", c.id)
	// fmt.Println("=============")
	// fmt.Printf("Args: \n%s\n", args.printableDetails())
	if Files[args.Filename] != nil {
		response.Success = true
	}
	// fmt.Printf("Response: \n%s\n", response.printableDetails())
	// fmt.Println("=============")
	// fmt.Printf("END FILE_EXISTS from %s\n", c.id)
	// fmt.Println("=============")
	return nil
}

// TODO Check all caches, not just owner
func (c *DFSClient) FileAvailable(args FileOpArgs, response *FileOpArgs) error {
	// fmt.Println("=============")
	// fmt.Printf("FILE_AVAILABLE from %s\n", c.id)
	// fmt.Println("=============")
	// fmt.Printf("Args: \n%s\n", args.printableDetails())
	// if such a file exists
	if Files[args.Filename] != nil {
		// for all clients in the history

	} else {
	}
	// fmt.Printf("Response: \n%s\n", response.printableDetails())
	// fmt.Println("=============")
	// fmt.Printf("END FILE_AVAILABLE from %s\n", c.id)
	// fmt.Println("=============")
	return nil
}

func (c *DFSClient) DownloadFile(args FileOpArgs, response *FileOpArgs) error {
	// fmt.Println("=============")
	// fmt.Printf("DOWNLOAD_FILE from %s\n", c.id)
	// fmt.Println("=============")
	// fmt.Printf("Args: \n%s\n", args.printableDetails())
	c.updateFile(args.Filename)
	// fmt.Printf("Response: \n%s\n", response.printableDetails())
	// fmt.Println("=============")
	// fmt.Printf("END DOWNLOAD_FILE from %s\n", c.id)
	// fmt.Println("=============")
	return nil
}

func (c *DFSClient) UMount(args FileOpArgs, response *FileOpArgs) error {
	// fmt.Println("=============")
	// fmt.Printf("UMOUNT from %s\n", c.id)
	// fmt.Println("=============")
	// fmt.Printf("Args: \n%s\n", args.printableDetails())
	// set client as disconnected
	c.connected = false
	// TODO close any open files
	response.Success = true
	// fmt.Printf("Response: \n%s\n", response.printableDetails())
	// fmt.Println("=============")
	// fmt.Printf("END UMOUNT from %s\n", c.id)
	// fmt.Println("=============")
	return nil
}

// ================
// Server Calls
// ================

// when complete, we assume the file in the cache is up to date.
func (c *DFSClient) updateFile(filename string) error {
	// fmt.Printf("Updating files for: %s\n", filename)
	// fmt.Printf("Files: %v\n", Files)
	// fmt.Printf("File %v\n", Files[filename].printableFileString())
	// for each chunk in file, use file and the client registry to call get Chunk for each to build a file.
	file := Files[filename]
	for chunkNum, vChunk := range file.VersionedChunks {
		// TODO: check not trivial
		// trivial: no chunks written
		// if trivial, nothing to update.
		if vChunk != nil {
			mostRecentId := vChunk.WriteVersionHistory[len(vChunk.WriteVersionHistory)-1]
			// unless chunk has id c.id
			if mostRecentId != c.id {
				// TODO: assuming they are still connected
				// update it
				client := Clients[mostRecentId]
				chunk, err := client.fetchChunk(filename, uint8(chunkNum))
				if err != nil {
					err = fmt.Errorf("Error while fetching chunk %d on %s: %v", chunkNum, filename, err)
					return err
				}
				err = c.setChunk(filename, uint8(chunkNum), chunk)
				if err != nil {
					err = fmt.Errorf("Error while setting chunk %d on %s: %v", chunkNum, filename, err)
					return err
				}
			}
		}
	}
	return nil
}

// writes a chunk to the cache
func (c *DFSClient) setChunk(filename string, chunkNum uint8, chunk Chunk) (err error) {
	args := ChunkOpArgs{
		Filename: filename,
		ChunkNum: chunkNum,
		Chunk:    chunk,
	}
	err = c.outgoingRPCConn.Call("DFSServer.SetChunk", args, nil)
	return err
}

// fetches a chunk from the cache on the client
func (c *DFSClient) fetchChunk(filename string, chunkNum uint8) (chunk Chunk, err error) {
	args := ChunkOpArgs{
		Filename: filename,
		ChunkNum: chunkNum,
		Chunk:    chunk,
	}
	response := ChunkOpArgs{}
	err = c.outgoingRPCConn.Call("DFSServer.FetchChunk", args, &response)
	return response.Chunk, err
}

func (c *DFSClient) fingerprint() (string, error) {
	if c.id == "" {
		print, err := fingerprinter.Generate()
		if err != nil {
			err = fmt.Errorf("Unable to fingerprint client: %v", err)
			return "", err
		}
		c.id = print
	}
	return c.id, nil
}

func (c *DFSClient) assignIdentity() error {
	err := c.outgoingRPCConn.Call("DFSServer.SetIdentity", c.id, nil)
	if err != nil {
		err = fmt.Errorf("Error while calling DFSServer.Identity: %v", err)
		return err
	}
	return nil
}

func (c *DFSClient) currentState() (state State, err error) {
	return c.state, nil
}

func (c *DFSClient) clean() (err error) {
	c.state = CLEAN
	return nil
}

func NewDFSClient(outgoingRPCConn *rpc.Client) (client DFSClient, err error) {
	client = DFSClient{}
	client.outgoingRPCConn = outgoingRPCConn
	return client, nil
}

// =============
// END DFSClient
// =============

func Serve(serverTCPAddr net.TCPAddr) (err error) {
	// fmt.Println("Server Starting Serve")
	listener, err := net.ListenTCP("tcp", &serverTCPAddr)
	for {
		// TODO: Make Parallel
		incomingConn, err := listener.Accept()
		if err != nil {
			err = fmt.Errorf("Error while listening: %v", err)
			return err
		}
		// Establish Reverse RPC connection
		rpcAddr, err := net.ResolveTCPAddr("tcp", incomingConn.RemoteAddr().String())
		if err != nil {
			err = fmt.Errorf("Error while resolving: %v", err)
			return err
		}
		rpcAddr.Port = 8090
		outgoingRPCConn, err := rpc.Dial("tcp", rpcAddr.String())
		if err != nil {
			err = fmt.Errorf("Error while dialing: %v", err)
			return err
		}
		newPort := -1
		err = outgoingRPCConn.Call("RPCInit.Setup", rpcAddr.IP.String(), &newPort)
		if err != nil || newPort == -1 {
			err = fmt.Errorf("Error while calling Setup new Port %d: %v", newPort, err)
			return err
		}
		outgoingRPCConn.Close()
		// fmt.Printf("New Port is: %d\n", newPort)
		rpcAddr.Port = newPort
		outgoingRPCConn, err = rpc.Dial("tcp", rpcAddr.String())
		// Identify Client
		id := "old"
		// fmt.Printf("Client at: %s\n", incomingConn.RemoteAddr().String())
		// TODO close nicely
		err = outgoingRPCConn.Call("DFSServer.CloseInit", newPort, nil)
		if err != nil {
			err = fmt.Errorf("Error while calling DFSServer.CloseInit: %v", err)
			return err
		}
		err = outgoingRPCConn.Call("DFSServer.Identify", serverTCPAddr.String(), &id)
		if err != nil {
			err = fmt.Errorf("Error while calling DFSServer.Identify: %v", err)
			return err
		}
		// fmt.Printf("Client gave id: %s\n", id)
		// Generate new ServerClient if new
		if id == "" || Clients[id] == nil {
			// Create new client
			newClient, err := NewDFSClient(outgoingRPCConn)
			// Fingerprint the new client
			id, err = newClient.fingerprint()
			// fmt.Printf("Print: %s\n", id)
			if err != nil {
				err = fmt.Errorf("Error while fingerprinting: %v", err)
				return err
			}
			// Send Fingerprint
			err = newClient.assignIdentity()
			if err != nil {
				err = fmt.Errorf("Error while assigning identity: %v", err)
				return err
			}
			// Register the new client with our server
			Clients[id] = &newClient
			if err != nil {
				err = fmt.Errorf("Error while creating new client object: %v", err)
				return err
			}
		}
		if Clients[id].connected {
			Clients[id].connected = false
		}
		Clients[id].connected = true
		// fmt.Printf("Clients: %v\n", Clients)
		// fmt.Printf("Id: %s\n", id)
		// fmt.Printf("Client: %v\n", Clients[id])
		activeClient := Clients[id]

		if activeClient.state == DIRTY {
			err := activeClient.clean()
			if err != nil {
				err = fmt.Errorf("Error while cleaning state: %v", err)
				return err
			}
		}
		rpcServer := rpc.NewServer()
		err = rpcServer.Register(activeClient)
		if err != nil {
			return fmt.Errorf("Unable to registerRPC: %v", err)
		}
		go rpcServer.ServeConn(incomingConn)
	}
}

// returns an ordered list of clients to try
func findBestClients(filename string, chunkNum uint8) (clients []*DFSClient, err error) {
	vChunks := Files[filename].VersionedChunks
	// check chunk exists
	if vChunks[chunkNum] == nil {
		return nil, fmt.Errorf("Given chunk is trivial!")
	}
	writeHist := vChunks[chunkNum].WriteVersionHistory
	openHist := vChunks[chunkNum].OpenVersionHistory
	clientIds := []string{}

	// by version, add writers and openers
	for version := len(writeHist) - 1; version >= 0; version-- {
		writerId := writeHist[version]
		if !contains(clientIds, writerId) {
			clientIds = append(clientIds, writerId)
		}
		openedIds := openHist[version]
		// check if anyone has opened it
		if openedIds != nil {
			for _, clientId := range *openedIds {
				if !contains(clientIds, clientId) {
					clientIds = append(clientIds, clientId)
				}
			}
		}
	}

	// remove any who are not connected.
	for _, id := range clientIds {
		if Clients[id].connected {
			clients = append(clients, Clients[id])
		}
	}
	return clients, nil
}

// we know that the chunk is non-trivial
func addClientToOpenVersionHistory(clientId string, filename string, chunkNum uint8) error {
	vChunk := Files[filename].VersionedChunks[chunkNum]
	currentVersion := len(vChunk.WriteVersionHistory) - 1
	vOpenHist := vChunk.OpenVersionHistory[currentVersion]
	if vOpenHist == nil {
		vOpenHist = &[]string{}
	}
	newVersionOpenHist := append(*vOpenHist, clientId)
	vChunk.OpenVersionHistory[currentVersion] = &newVersionOpenHist
	return nil
}

func contains(a []string, b string) bool {
	for _, s := range a {
		if b == s {
			return true
		}
	}
	return false
}

func fileModeAsString(mode FileMode) (modeStr string) {
	switch mode {
	case READ:
		modeStr = "READ"
	case WRITE:
		modeStr = "WRITE"
	default:
		modeStr = "UNKNOWN"
	}
	return modeStr
}
