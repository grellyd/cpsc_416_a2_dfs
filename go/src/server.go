package main

import (
	"fmt"
	"os"
	"serverlib/cli"
	"serverlib/dfs_server"
)

func main() {
	serverTCPAddr, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	err = dfs_server.Serve(serverTCPAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
