package addrparse

import (
	"net"
	"testing"
)

func TestTCP(t *testing.T) {
	var tests = []struct {
		input  string
		output net.TCPAddr
	}{
		{
			"192.168.0.1:5000",
			net.TCPAddr{
				IP:   net.IPv4(byte(192), byte(168), byte(0), byte(1)),
				Port: 5000,
				Zone: "",
			},
		},
		{
			"127.134.0.1:3030",
			net.TCPAddr{
				IP:   net.IPv4(byte(127), byte(134), byte(0), byte(1)),
				Port: 3030,
				Zone: "",
			},
		},
		{
			"198.162.33.54:5555",
			net.TCPAddr{
				IP:   net.IPv4(byte(198), byte(162), byte(33), byte(54)),
				Port: 5555,
				Zone: "",
			},
		},
	}
	for _, test := range tests {
		result, err := TCP(test.input)
		if err != nil {
			t.Errorf("Bad Exit: \"TCP(%s)\" produced err: %v", test.input, err)
		}
		if !result.IP.Equal(test.output.IP) || result.Port != test.output.Port || result.Zone != test.output.Zone {
			t.Errorf("Bad Exit: \"TCP(%s)\" = %q not %q", test.input, result, test.output)
		}
	}
}

func TestUDP(t *testing.T) {
	var tests = []struct {
		input  string
		output net.UDPAddr
	}{
		{
			"192.168.0.1:5000",
			net.UDPAddr{
				IP:   net.IPv4(byte(192), byte(168), byte(0), byte(1)),
				Port: 5000,
				Zone: "",
			},
		},
		{
			"127.134.0.1:3030",
			net.UDPAddr{
				IP:   net.IPv4(byte(127), byte(134), byte(0), byte(1)),
				Port: 3030,
				Zone: "",
			},
		},
		{
			"198.162.33.54:5555",
			net.UDPAddr{
				IP:   net.IPv4(byte(198), byte(162), byte(33), byte(54)),
				Port: 5555,
				Zone: "",
			},
		},
	}
	for _, test := range tests {
		result, err := UDP(test.input)
		if err != nil {
			t.Errorf("Bad Exit: \"UDP(%s)\" produced err: %v", test.input, err)
		}
		if !result.IP.Equal(test.output.IP) || result.Port != test.output.Port || result.Zone != test.output.Zone {
			t.Errorf("Bad Exit: \"UDP(%s)\" = %q not %q", test.input, result, test.output)
		}
	}
}

func TestBadUDP(t *testing.T) {
	var tests = []struct {
		input        string
		output       net.UDPAddr
		error_string string
	}{
		{
			"192..0.1:5000",
			net.UDPAddr{},
			"address parsing failed: 192..0.1:5000 as UDP: ip parsing failed: 192..0.1 as ip: strconv.Atoi: parsing \"\": invalid syntax",
		},
		{
			"127.0.1:3030",
			net.UDPAddr{},
			"address parsing failed: 127.0.1:3030 as UDP: ip parsing failed: 127.0.1 as ip",
		},
		{
			"0:5555",
			net.UDPAddr{},
			"address parsing failed: 0:5555 as UDP: ip parsing failed: 0 as ip",
		},
	}
	for _, test := range tests {
		result, err := UDP(test.input)
		if result.String() != test.output.String() {
			t.Errorf("Bad Exit: \"UDP(%s)\" instead of %v, test produced a result: %v ", test.input, test.output, result)
		}
		if err == nil {
			t.Errorf("Bad Exit: \"UDP(%s)\" produced no err: %v", test.input, err)
		}
		if err.Error() != test.error_string {
			t.Errorf("Bad Exit: \"UDP(%s)\" produced incorrect error, %s vs. %s", test.input, err, test.error_string)
		}
	}
}
