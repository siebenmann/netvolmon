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
	"os"

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
func (im ipMap) add(ip, netdev string) {
	v, ok := im[ip]
	if ok {
		im[ip] = append(v, netdev)
	} else {
		im[ip] = []string{netdev}
	}
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

// Match 'me' and try to translate it to an IP address via host lookup,
// then find the IP address(es) in our devices.
// TODO: try to pick one primary address? That gets complicated.
func matchMe(devpat string, ipmap ipMap, tgt set) bool {
	if devpat != "me" {
		return false
	}
	hn, err := os.Hostname()
	if err != nil {
		return false
	}
	addrs, err := net.LookupHost(hn)
	if err != nil {
		return false
	}
	matched := false
	for _, a := range addrs {
		if v, ok := ipmap[a]; ok {
			tgt.addlist(v)
			matched = true
		}
	}
	return matched
}

func matchNetNames(devpat string, ipmap ipMap, tgt set) bool {
	if cidr, ok := cslabNetNames[devpat]; ok {
		return cidrIPMatch(cidr, ipmap, tgt)
	}
	if slist, ok := cslabMultiNames[devpat]; ok {
		// we match if any one of the multi-name matched,
		// so we can have entries like 'blue' for 'net3 and/or
		// net5'.
		matched := false
		for _, name := range slist {
			cidr, ok := cslabNetNames[name]
			if !ok {
				// TODO: really this is a fatal error
				return false
			}
			if cidrIPMatch(cidr, ipmap, tgt) {
				matched = true
			}
		}
		return matched
	}
	return false
}

// expandDevList takes a list of network device names from the command
// line, plus the starting stats structure, and attempts to find actual
// network device names for all of the arguments. It does various sorts
// of matching.
//
// BUGS: we assume the network device name list from oldst matches the
// network device names that net.Interfaces() will return in Interfaces
// structures.
func expandDevList(devices []string, oldst Stats, exlist []string) []string {
	// We cannot simply put matching devices in a list, because
	// multiple command line arguments may match an overlapping
	// set of devices and we don't want repeated device names.
	// So we must put them in a set (here a string-based map)
	// and then turn them into an array at the end.
	nk := make(set)

	devs := oldst.members()

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
		//
		// We deliberately start out with our special magic
		// matches.
		if matchMe(k, netinfo.ipmap, nk) ||
			matchNetNames(k, netinfo.ipmap, nk) ||
			globMatch(k, devs, nk) ||
			ipMatch(k, netinfo.ipmap, nk) ||
			cidrIPMatch(k, netinfo.ipmap, nk) ||
			globIPMatch(k, netinfo.ipmap, nk) {
			continue
		}

		// No match? Fail here.
		log.Fatalf("device specifier '%s' doesn't seem to exist or match anything", k)
	}

	// Turn our 'nk' set of matched network device names into a
	// sorted list, first removing excluded devices.
	for _, k := range exlist {
		nk.remove(k)
	}
	return nk.members()
}
