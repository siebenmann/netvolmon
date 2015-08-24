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
	"time"
)

// A DevStat represents a moment in time snapshot of a network device's
// current statistics.
type DevStat struct {
	When     time.Time
	RBytes   uint64
	TBytes   uint64
	RPackets uint64
	TPackets uint64
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

// Generate a set of deltas between two Stats. Devices can appear and
// disappear; only devices that are in both Stats are included in the
// deltas. We skip any devices that appear to have had counter overflow
// and any devices that appear to be totally inactive, with no bytes
// ever transmitted or received.
func genDeltas(oldinfo, newinfo Stats) Deltas {
	d := make(Deltas)
	for devname, nv := range newinfo {
		// Skip interfaces that seem to be totally inactive.
		if nv.RBytes == 0 && nv.TBytes == 0 {
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

const (
	kB = 1024
	mB = kB * 1024

	// HMS is our timestamp format for -T. It omits the date for space.
	// This is not expected to usually matter.
	HMS = "15:04:05"
)

var showTimestamp = true
var showZero = true
var incLo = false
var duration = time.Second

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

func processLoop(devices []string) {
	var keys []string

	oldst := make(Stats)
	e := oldst.Fill()
	if e != nil {
		log.Fatal("error on initial filling: ", e)
	}

	if len(devices) > 0 {
		keys = expandDevList(devices, oldst)
	}

	for {
		time.Sleep(duration)
		newst := make(Stats)
		e = newst.Fill()
		if e != nil {
			log.Fatal("error refilling: ", e)
		}

		dt := genDeltas(oldst, newst)

		if len(devices) == 0 {
			keys = make([]string, len(dt))
			i := 0
			for k := range dt {
				keys[i] = k
				i++
			}
			sort.Strings(keys)
		}

		for _, k := range keys {
			if !incLo && k == "lo" {
				continue
			}
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

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\t%s [options] [network-dev [network-dev ...]]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nDefault is to report on all network devices that have seen traffic.\nNetwork device names can include glob patterns, eg 'enp*f*'.\n")
}

func main() {
	var usekb bool

	log.SetPrefix("netvolmon: ")
	log.SetFlags(0)

	flag.BoolVar(&incLo, "l", false, "don't report on 'lo' device")
	flag.BoolVar(&showTimestamp, "T", false, "show timestamps")
	flag.BoolVar(&showZero, "z", false, "show devices with no activity this period")
	flag.DurationVar(&duration, "d", time.Second, "`delay` between reports")
	flag.BoolVar(&usekb, "k", false, "report in KB/s instead of MB/s")

	flag.Usage = usage
	flag.Parse()

	if usekb {
		bwUnits = "KB/s"
		bwDiv = kB
	}
	if flag.NArg() > 0 {
		incLo = true
	}

	processLoop(flag.Args())
}
