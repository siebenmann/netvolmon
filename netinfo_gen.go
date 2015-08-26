//
// Generic network interface information, using net.Interfaces() et al
// Unfortunately the Go standard library only supports this on some
// platforms.
//
// +build !solaris

package main

import (
	"net"
)

func setupNetinfo() error {
	ints, e := net.Interfaces()
	if e != nil {
		return e
	}

	for _, i := range ints {
		if (i.Flags & net.FlagLoopback) > 0 {
			netinfo.loopbacks.add(i.Name)
		}
		if (i.Flags & net.FlagPointToPoint) > 0 {
			netinfo.pointtopoint.add(i.Name)
		}
		netinfo.ifaces = append(netinfo.ifaces, i.Name)

		addrs, e := i.Addrs()
		if e != nil {
			continue
		}
		for _, a := range addrs {
			// I HATE YOUR DOCUMENTATION
			if a.Network() != "ip+net" {
				continue
			}
			astr := a.String()
			// We don't care about and can't use the CIDR,
			// but we want the IP address.
			ip, _, e := net.ParseCIDR(astr)
			if e != nil {
				continue
			}
			netinfo.ipmap.add(ip.String(), i.Name)
		}
	}
	return nil
}
