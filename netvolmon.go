//
// Report on network device bandwidth and packet count, in per-second
// numbers, for however many network devices you want to at once.
// Reports can be in MB/s or KB/s and can include timestamps. Network
// devices can be 'all active devices', specific devices, or wildcards
// (because Chris is lazy).
//
// BUGS: Linux only, because so far there doesn't seem to be Golang
// support for Solaris/Illumos kstat(s).
//
// Author: Chris Siebenmann
//
// Copyright: GPL v3
//
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// low rent sets of strings.
type set map[string]struct{}

func (s set) add(dev string) {
	s[dev] = struct{}{}
}

func (s set) addlist(lst []string) {
	for _, k := range lst {
		s.add(k)
	}
}

func (s set) remove(dev string) {
	delete(s, dev)
}

func (s set) members() []string {
	keys := make([]string, len(s))
	i := 0
	for k := range s {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

func (s set) isin(name string) bool {
	_, ok := s[name]
	return ok
}

// netinfo is our central point for network interface information
// it is filled in setupNetinfo(), which is system-specific.
type netInfo struct {
	ipmap        ipMap
	ifaces       []string
	loopbacks    set
	pointtopoint set
}

var netinfo netInfo

//
//

// A DevStat represents a moment in time snapshot of a network device's
// current statistics.
type DevStat struct {
	When     time.Time
	RBytes   uint64
	TBytes   uint64
	RPackets uint64
	TPackets uint64
	// TODO: error stats?
}

// A DevDelta represents the difference between two DevStats. It has
// the same fields, plus a Delta that is the time difference between
// them.
type DevDelta struct {
	DevStat
	Delta time.Duration
}

// subChecked subtracts two numbers if it looks like there hasn't
// been a counter overflow. It preserves a running flag of good
// vs bad if its particular check is good, otherwise returns 0
// and false.
func subChecked(a, b uint64, good bool) (uint64, bool) {
	if a <= b {
		return b - a, good
	}
	return 0, false
}

// Delta computes the change between two DevStats and returns a delta
// along with an indicator if it's good. Deltas are bad if there appears
// to be counter rollovers between the first and second stats.
func Delta(oldst, newst *DevStat) (DevDelta, bool) {
	good := true

	n := DevDelta{}
	n.Delta = newst.When.Sub(oldst.When)
	n.When = newst.When
	n.RBytes, good = subChecked(oldst.RBytes, newst.RBytes, good)
	n.TBytes, good = subChecked(oldst.TBytes, newst.TBytes, good)
	n.RPackets, good = subChecked(oldst.RPackets, newst.RPackets, good)
	n.TPackets, good = subChecked(oldst.TPackets, newst.TPackets, good)
	return n, good
}

// Stats represents a collection of device stats, one entry per device.
//
// Concrete system-dependent support for this creates a .Fill() method
// that fills a Stats map with a point in time snapshot of available
// network device stats. So far only Linux is supported.
type Stats map[string]DevStat

// Deltas represents the delta between two device stats, one entry per device
type Deltas map[string]DevDelta

// oh for generic functions. this is cut and paste but that's life.
func (s Stats) members() []string {
	keys := make([]string, len(s))
	i := 0
	for k := range s {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

func (d Deltas) members() []string {
	keys := make([]string, len(d))
	i := 0
	for k := range d {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

// Generate a set of deltas between two Stats. Devices can appear and
// disappear; only devices that are in both Stats are included in the
// deltas. We skip any devices that appear to have had counter overflow
// and any devices that appear to be totally inactive, with no bytes
// ever transmitted or received.
func genDeltas(oldinfo, newinfo Stats) Deltas {
	d := make(Deltas)
	for devname, nv := range newinfo {
		// Skip interfaces that seem to be totally inactive.
		// Our standard for 'totally inactive' is no bytes
		// received, because systems can try to send stuff
		// out on dead interfaces for reasons.
		//
		// It isn't sufficient to check the interface's
		// FlagUp, because we can have up but totally
		// inactive interfaces.
		if nv.RBytes == 0 {
			continue
		}
		ov, ok := oldinfo[devname]
		if !ok {
			continue
		}
		delta, good := Delta(&ov, &nv)
		if good {
			d[devname] = delta
		}
	}
	return d
}

//
//
const (
	kB = 1024
	mB = kB * 1024

	// HMS is our timestamp format for -T. It omits the date for space.
	// This is not expected to usually matter.
	HMS = "15:04:05"
)

var showTimestamp bool
var showZero bool
var incLo bool
var duration time.Duration

var bwUnits = "MB/s"
var bwDiv = mB

// printDelta prints the per-second rates for a given device given its
// DevDelta. Bandwidth is scaled.
func printDelta(devname string, dt DevDelta) {
	persec := float64(dt.Delta) / float64(time.Second)
	persecbytes := persec * float64(bwDiv)

	if showTimestamp {
		fmt.Printf("%-8s %8s ", devname, dt.When.Format(HMS))
	} else {
		fmt.Printf("%-8s ", devname)
	}
	fmt.Printf("%6.2f RX %6.2f TX (%s)   packets/sec: %5.0f RX %5.0f TX\n",
		float64(dt.RBytes)/persecbytes,
		float64(dt.TBytes)/persecbytes,
		bwUnits,
		float64(dt.RPackets)/persec,
		float64(dt.TPackets)/persec)
}

func processLoop(devices []string, report bool, exlist []string) {
	var keys []string

	oldst := make(Stats)
	e := oldst.Fill()
	if e != nil {
		log.Fatal("error on initial filling: ", e)
	}

	excludes := make(set)
	excludes.addlist(exlist)

	if len(devices) > 0 {
		keys = expandDevList(devices, oldst, exlist)

		// With -x/-P, we might wind up eliminating all devices
		// to monitor. We'd better check that explicitly.
		if len(keys) == 0 {
			log.Fatal("wound up with no devices to monitor!")
		}
	} else {
		// set up keys for the report flag
		// we must filter inactive devices by hand.
		// TODO: this is probably a code smell
		// Make it a DevStat method?
		keys = make([]string, 0, len(oldst))
		for k, v := range oldst {
			if (v.RBytes == 0) ||
				(!incLo && netinfo.loopbacks.isin(k)) ||
				excludes.isin(k) {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// excludes can theoretically take out all active
		// network devices even in the 'monitor it all' case.
		// in theory we could have a new network device appear;
		// in practice, well, we error out here.
		if len(keys) == 0 {
			log.Fatal("wound up with no devices to monitor!")
		}
	}

	// Report on what devices we'd use.
	if report {
		fmt.Printf("netvolmon: devices would be:")
		for _, k := range keys {
			fmt.Printf(" %s", k)
		}
		fmt.Printf("\n")
		return
	}

	for {
		time.Sleep(duration)
		newst := make(Stats)
		e = newst.Fill()
		if e != nil {
			log.Fatal("error refilling: ", e)
		}

		dt := genDeltas(oldst, newst)

		// Without explicit devices specified, we report on
		// whatever is available on each iteration. This may
		// include newly appearing devices, which is why we
		// don't precalculate the keys list.
		if len(devices) == 0 {
			keys = dt.members()
		}

		for _, k := range keys {
			if !incLo && netinfo.loopbacks.isin(k) {
				continue
			}
			if excludes.isin(k) {
				continue
			}

			// We might not have stats for some device
			// specified on the command line (perhaps
			// it disappeared).
			v, ok := dt[k]
			if !ok {
				continue
			}

			if !showZero && v.RBytes == 0 && v.TBytes == 0 {
				continue
			}
			printDelta(k, v)
		}
		oldst = newst
	}
}

var noteStr = `
Default is to report on all network devices that have received traffic.

Network device names can include shell glob patterns (eg 'enp*f*'),
interface IP addresses, wildcarded IP addresses (eg '127.*'), CIDR
netblocks (match any interface with an address in the netblock) and a
few special names like 'me' (which tries to do an IP address lookup on
the hostname and go from there).
`

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\t%s [options] [network-dev [network-dev ...]] [seconds]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, noteStr)
}

func main() {
	var usekb bool
	var report bool
	var exclude string
	var noPtP bool

	log.SetPrefix("netvolmon: ")
	log.SetFlags(0)

	flag.BoolVar(&incLo, "l", false, "when reporting on everything, report on loopback too")
	flag.BoolVar(&showTimestamp, "T", false, "include timestamps in output")
	flag.BoolVar(&showZero, "z", false, "show devices even if they have no activity this period")
	flag.DurationVar(&duration, "d", time.Second, "`delay` between reports")
	flag.BoolVar(&usekb, "k", false, "report in KB/s instead of MB/s")
	flag.BoolVar(&report, "R", false, "just report what devices we'd monitor")
	// TODO: this is kind of a hack.
	flag.StringVar(&exclude, "x", "", "`devices` to specifically exclude (comma-separated)")
	flag.BoolVar(&noPtP, "P", false, "exclude all point to point devices")

	flag.Usage = usage
	flag.Parse()

	if usekb {
		bwUnits = "KB/s"
		bwDiv = kB
	}

	// Very special hack: a single trailing integer argument is
	// interpreted as a duration in seconds.
	args := flag.Args()
	if len(args) > 0 {
		l := len(args) - 1
		// We don't bother trying to limit the size of the
		// duration via the #-of-bits argument here.
		dur, ok := strconv.ParseUint(args[l], 0, 64)
		if ok == nil && dur > 0 {
			duration = time.Second * time.Duration(dur)
			args = args[:l]
		}
	}

	if len(args) > 0 {
		incLo = true
	}

	// We assume that loopbacks and point to point devices don't
	// appear dynamically. This is the best we can do for reasons.
	netinfo.ipmap = make(ipMap)
	netinfo.loopbacks = make(set)
	netinfo.pointtopoint = make(set)
	e := setupNetinfo()
	if e != nil {
		log.Fatal("error on network info setup: ", e)
	}

	exlist := strings.Split(exclude, ",")
	// TODO: all of this hackery around various sorts of
	// exclusions is a code smell.
	if noPtP {
		for _, iname := range netinfo.pointtopoint.members() {
			exlist = append(exlist, iname)
		}
	}

	processLoop(args, report, exlist)
}
