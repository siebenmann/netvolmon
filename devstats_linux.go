//
// Linux implementation of obtaining a point in time snapshot of network
// device activity. We get all information by reading /proc/net/dev.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Maximum size of /proc/net/dev before we throw up our hands. This is
// way big, but.
const MAXSIZE = (128 * 1024)

func getInt(field string, e error) (uint64, error) {
	i, err := strconv.ParseUint(field, 10, 64)
	if err != nil {
		return i, err
	}
	return i, e
}

func parseLine(line string) (string, DevStat, error) {
	st := DevStat{}
	fields := strings.Fields(line)
	// We expect 17 fields.
	if len(fields) != 17 {
		return "", st, fmt.Errorf("incorrect number of fields: %d in '%s'", len(fields), line)
	}
	devname := strings.TrimSuffix(fields[0], ":")
	var rerr error
	st.RBytes, rerr = getInt(fields[1], rerr)
	st.RPackets, rerr = getInt(fields[2], rerr)
	st.TBytes, rerr = getInt(fields[9], rerr)
	st.TPackets, rerr = getInt(fields[10], rerr)
	return devname, st, rerr
}

// Fill fills a Stats map with current network stats for all known
// network devices.
func (s Stats) Fill() error {
	// Read all of /proc/net/dev's current state in one request,
	// so all measurements are in sync.
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return err
	}
	data := make([]byte, MAXSIZE)
	when := time.Now()
	count, err := file.Read(data)
	if err != nil {
		file.Close()
		return err
	}
	file.Close()

	// Sanity check the results for either a huge file or an empty
	// one.
	if count >= MAXSIZE {
		return errors.New("/proc/net/dev is too big, over MAXSIZE")
	}
	if count == 0 {
		return errors.New("read 0 bytes from /proc/net/dev")
	}

	lines := bytes.Split(data[:count], []byte("\n"))
	// The first two lines are headers. Normally we should have
	// at least a 'lo:' entry as well, so we error out if it
	// seems to be missing.
	if len(lines) < 3 {
		return errors.New("no devices in /proc/net/dev")
	}

	for _, line := range lines[2:] {
		if len(line) == 0 {
			continue
		}
		devname, devst, err := parseLine(string(line))
		if err != nil {
			return err
		}
		devst.When = when
		s[devname] = devst
	}

	// No problems, we're done.
	return nil
}
