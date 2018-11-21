package cli

import (
	"fmt"
	"net"
	"serverlib/cli/addrparse"
)

func ParseArgs(args []string) (clientTCPAddr net.TCPAddr, err error) {
	clientTCPAddr, err = addrparse.TCP(args[0])
	if err != nil {
		return net.TCPAddr{}, fmt.Errorf("cli failed: %v", err)
	}
	return clientTCPAddr, nil
}
