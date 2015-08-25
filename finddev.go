//
// Functions to find and match network device names from the command line.
// This matches entirely too many things:
//
// - plain network device names
// - globbed network device names
// - ip addresses (which must exactly match an IP address of one or more
//   interfaces)
// - CIDR netblocks, which are matched against the IP addresses of
//   interfaces
// - wildcarded IP address patterns, like '127.*'
//
// BUGS: desperately needs tests and refactoring

package main

import (
	"log"
	"net"

	"github.com/ryanuber/go-glob"
)

// Match a glob pattern against the names of network devices.
// We take the target map to add entries to because we may match multiple
// entries. In fact that's kind of the default case.
func globMatch(devpat string, netdevs []string, tgt set) bool {
	matched := false
	for _, dev := range netdevs {
		if glob.Glob(devpat, dev) {
			matched = true
			tgt.add(dev)
		}
	}
	return matched
}

// ----
// IP based matching

// ipMap maps IP addresses to *arrays* of network devices, because an
// IP address can be attached to more than one network device (yes,
// really).
type ipMap map[string][]string

// add adds an IP/device pairing to the map.
func (i ipMap) add(ip, netdev string) {
	v, ok := i[ip]
	if ok {
		i[ip] = append(v, netdev)
	} else {
		i[ip] = []string{netdev}
	}
}

// Create an ipMap for all interfaces on the system.
func getNetworks() ipMap {
	ipmap := make(ipMap)
	ints, e := net.Interfaces()
	if e != nil {
		return ipmap
	}
	for _, i := range ints {
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
			ipmap.add(ip.String(), i.Name)
		}
	}
	return ipmap
}

// ipMatch is given an IP address (or a potential one) and finds it
// in the ipMap to add all network devices associated with that IP.
// eg '127.0.0.1' -> 'lo'
// This will always only match a single ipmap entry, but that entry
// might have multiple devices associated with it.
func ipMatch(devpat string, ipmap ipMap, tgt set) bool {
	ip := net.ParseIP(devpat)
	if ip == nil {
		return false
	}
	if v, ok := ipmap[ip.String()]; ok {
		tgt.addlist(v)
		return true
	}
	return false
}

// globIPMatch is given an IP address glob and matches it against the
// IP addresses associated with network devices, adding all that match.
// eg '127.*' -> 'lo'
func globIPMatch(devpat string, ipmap ipMap, tgt set) bool {
	matched := false
	for k, v := range ipmap {
		if glob.Glob(devpat, k) {
			matched = true
			tgt.addlist(v)
		}
	}
	return matched
}

// cidrIPMatch is given a CIDR and matches it against the IP addresses
// associated with network devices, adding all that match.
// eg '127.0.0.0/8' -> 'lo'.
func cidrIPMatch(devpat string, ipmap ipMap, tgt set) bool {
	matched := false
	_, cidr, err := net.ParseCIDR(devpat)
	if err != nil {
		return false
	}
	for k, v := range ipmap {
		ip := net.ParseIP(k)
		if cidr.Contains(ip) {
			tgt.addlist(v)
			matched = true
		}
	}
	return matched
}

// expandDevList takes a list of network device names from the command
// line, plus the starting stats structure, and attempts to find actual
// network device names for all of the arguments. It does various sorts
// of matching.
//
// BUGS: we assume the network device name list from oldst matches the
// network device names that net.Interfaces() will return in Interfaces
// structures.
func expandDevList(devices []string, oldst Stats) []string {
	// We cannot simply put matching devices in a list, because
	// multiple command line arguments may match an overlapping
	// set of devices and we don't want repeated device names.
	// So we must put them in a set (here a string-based map)
	// and then turn them into an array at the end.
	nk := make(set)

	devs := oldst.members()
	ipmap := getNetworks()

	// Try multiple strategies to find network devices for each
	// command line argument.
	for _, k := range devices {
		// The simplest one is a plain network device name, which
		// we try to match in the stats map.
		_, ok := oldst[k]
		if ok {
			nk.add(k)
			continue
		}

		// Try all of our complicated matching. The order is
		// basically from what we think is probably the cheapest
		// to the most expensive. It's probably wrong.
		//
		// All matchers return 'true' if they match something,
		// 'false' otherwise. First one to hit wins.
		if globMatch(k, devs, nk) ||
			ipMatch(k, ipmap, nk) ||
			cidrIPMatch(k, ipmap, nk) ||
			globIPMatch(k, ipmap, nk) {
			continue
		}

		// No match? Fail here.
		log.Fatalf("device specifier '%s' doesn't seem to exist or match anything", k)
	}

	// Turn our 'nk' set of matched network device names into a
	// sorted list.
	return nk.members()
}
