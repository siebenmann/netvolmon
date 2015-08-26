//
// Generic network interface information generated from getifaddrs(),
// at least the version of it on Solaris. This is necessary because
// the Go standard library on Solaris/Illumos/etc does not currently
// implement the net.Interface related stuff and always returns 'no
// information' results.
//
// The need for -lsendfile is a ??!, but I got link failures without
// it. No, I don't understand it either.

package main

// #cgo LDFLAGS: -lsocket -lnsl -lsendfile
//
// #include <sys/types.h>
// #include <sys/socket.h>
// #include <ifaddrs.h>
// #include <stdlib.h>
// #include <arpa/inet.h>
// #include <netinet/in.h>
// #include <net/if.h>
//
import "C"
import (
	"fmt"
	"unsafe"
)

// We currently only set up information for interfaces with IPv4
// addresses associated with them.
//
// Solaris's getifaddrs() (currently) only returns results for
// interfaces that have been configured with IP addresses.
// Inactive interfaces are thus silently omitted, unlike on eg
// Linux.
//
// TODO: I should figure out some scheme of first trying the
// net.Interfaces() stuff, so that when Go 1.x finally gets
// support for it on Solaris we'll automatically start to
// use it.
func setupNetinfo() error {
	var ifap *C.struct_ifaddrs

	rc, err := C.getifaddrs(&ifap)
	if rc != 0 || err != nil {
		C.freeifaddrs(ifap)
		return err
	}
	ifaces := make(set)

	// Note that the ifap list has one entry *per IP*; if a single
	// interface has multiple IPs associated with it, via eg
	// aliases, the interface will show up multiple times as we
	// traverse the list. This is unlike net.Interfaces().
	for fi := ifap; fi != nil; fi = fi.ifa_next {
		// sorry, I only deal with IPv4 right now.
		if fi.ifa_addr.sa_family != C.AF_INET {
			continue
		}
		iname := C.GoString(fi.ifa_name)
		ifaces.add(iname)

		if (fi.ifa_flags & C.IFF_LOOPBACK) > 0 {
			netinfo.loopbacks.add(iname)
		}
		if (fi.ifa_flags & C.IFF_POINTOPOINT) > 0 {
			netinfo.pointtopoint.add(iname)
		}

		// Get the IPv4 address associated with this entry.
		// We set it up as a string.
		//
		// Reverse engineering what the sin_addr field is
		// called by CGo was a pain in the ass. Thank goodness
		// for %#v is all I can say; CGo apparently takes the
		// leading _ off what is really '_S_un' for its own
		// reasons.
		//
		// Because this is a union, CGo sets it up as a
		// uint8 buffer. This is very convenient for us because
		// we want to interpret it that way anyways so we can
		// just Sprintf() the bytes into a string.
		t := (*C.struct_sockaddr_in)(unsafe.Pointer(fi.ifa_addr)).sin_addr.S_un
		ipstr := fmt.Sprintf("%d.%d.%d.%d", t[0], t[1], t[2], t[3])

		netinfo.ipmap.add(ipstr, iname)
	}
	C.freeifaddrs(ifap)

	netinfo.ifaces = ifaces.members()
	return nil
}
