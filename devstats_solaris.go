//
// Solaris implementation of obtaining a point in time snapshot of
// network device activity. We get all of our information through
// Solaris kstats, which we read via my package for doing this.
//

package main

import (
	"fmt"
	"time"

	"github.com/siebenmann/go-kstat"
)

// We hold our kstat perpetually open.
// In theory this leaks memory, but.
var khandle *kstat.Token

// getUint gets a Uint64 named kstat if there have been no errors to
// date, and otherwise rolls errors forward (returning 0 as the
// kstat's value).
func getUint(ks *kstat.KStat, name string, err error) (uint64, error) {
	if err != nil {
		return 0, err
	}
	ns, err := ks.GetNamed(name)
	if err != nil {
		return 0, fmt.Errorf("getting %s from %s error: %s", name, ks, err)
	}
	if ns.Type != kstat.Uint64 {
		return 0, fmt.Errorf("kstat %s is a %s not a uint64", ns, ns.Type)
	}
	return ns.UintVal, nil
}

func statsFor(iname string) (*DevStat, error) {
	ks, err := khandle.Lookup("link", 0, iname)
	// If we cannot get link stats for a device for some reason,
	// we consider it a non-fatal error. This applies to eg loopback
	// devices, which in Solaris do not have link stats.
	if err != nil {
		return nil, nil
	}
	// force us to have current information.
	err = ks.Refresh()
	if err != nil {
		return nil, fmt.Errorf("refreshing %s: %s", ks, err)
	}

	st := DevStat{}
	st.When = time.Now()
	// Let's thank Solaris for the 'ipackets' vs 'rbytes' inconsistency
	// here.
	st.RBytes, err = getUint(ks, "rbytes64", err)
	st.RPackets, err = getUint(ks, "ipackets64", err)
	st.TBytes, err = getUint(ks, "obytes64", err)
	st.TPackets, err = getUint(ks, "opackets64", err)
	return &st, err
}

// Fill stats with current information for all available devices.
func (s Stats) Fill() error {
	var err error
	// TODO: we should have an init function instead of hijacking
	// things this way.
	if khandle == nil {
		khandle, err = kstat.Open()
		if err != nil {
			return err
		}
	}

	//
	for _, iname := range netinfo.ifaces {
		devst, err := statsFor(iname)
		if err != nil {
			return err
		}
		// no stats available for this device, skip it.
		// this may be a mistake.
		if devst == nil {
			continue
		}
		s[iname] = *devst
	}
	return nil
}
