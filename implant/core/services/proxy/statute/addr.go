package statute

import (
	"fmt"
	"net"
	"strconv"
)

// AddrSpec is used to return the target AddrSpec
// which may be specified as IPv4, IPv6, or a FQDN
type AddrSpec struct {
	FQDN string
	IP   net.IP
	Port int
	// private stuff set when Request parsed
	AddrType byte
}

// String returns a string suitable to dial; prefer returning IP-based
// address, fallback to FQDN
func (sf *AddrSpec) String() string {
	// 🔧 修复：添加空指针检查
	if sf == nil {
		return "<nil>"
	}
	if len(sf.IP) != 0 {
		return net.JoinHostPort(sf.IP.String(), strconv.Itoa(sf.Port))
	}
	if sf.FQDN != "" {
		return net.JoinHostPort(sf.FQDN, strconv.Itoa(sf.Port))
	}
	return "<empty>"
}

// Address returns a string which may be specified
// if IPv4/IPv6 will return < ip:port >
// if FQDN will return < domain ip:port >
// Note: do not used to dial, Please use String
func (sf AddrSpec) Address() string {
	// 🔧 修复：添加空值检查
	if sf.FQDN != "" {
		if sf.IP != nil {
			return fmt.Sprintf("%s (%s):%d", sf.FQDN, sf.IP, sf.Port)
		} else {
			return fmt.Sprintf("%s:<unresolved>:%d", sf.FQDN, sf.Port)
		}
	}
	if sf.IP != nil {
		return fmt.Sprintf("%s:%d", sf.IP, sf.Port)
	}
	return fmt.Sprintf("<empty>:%d", sf.Port)
}

// ParseAddrSpec parse addr(host:port) to the AddrSpec address
func ParseAddrSpec(addr string) (as AddrSpec, err error) {
	var host, port string

	host, port, err = net.SplitHostPort(addr)
	if err != nil {
		return
	}
	as.Port, err = strconv.Atoi(port)
	if err != nil {
		return
	}

	ip := net.ParseIP(host)
	if ip4 := ip.To4(); ip4 != nil {
		as.AddrType, as.IP = ATYPIPv4, ip
	} else if ip6 := ip.To16(); ip6 != nil {
		as.AddrType, as.IP = ATYPIPv6, ip
	} else {
		as.AddrType, as.FQDN = ATYPDomain, host
	}
	return
}
