// Provide a mapping from customary local network names to CIDR netblocks
// for them.

package main

// name to CIDR
var cslabNetNames = map[string]string{
	"net3": "128.100.3.0/24",
	"net5": "128.100.5.0/24",

	"dev2": "192.168.151.0/24",
	"core": "192.168.66.0/24",

	"iscsi1": "192.168.101.0/24",
	"iscsi2": "192.168.102.0/24",

	"red":  "172.17.0.0/16",
	"vpn":  "172.29.0.0/16",
	"wifi": "172.31.0.0/16",
}

// abstract name to specific names, which must be in cslabNetNames.
// Sorry, no mixed names + CIDRs.
var cslabMultiNames = map[string][]string{
	"iscsi": {"iscsi1", "iscsi2"},
	"blue":  {"net3", "net5"},
}
