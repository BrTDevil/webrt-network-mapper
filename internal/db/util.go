package db

import (
	"net/netip"
	"sort"
)

// sortByIP ordonează numeric după IP; cele fără IP ajung la final.
func sortByIP(ds []Device) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, errA := netip.ParseAddr(ds[i].LastIP)
		b, errB := netip.ParseAddr(ds[j].LastIP)
		switch {
		case errA != nil && errB != nil:
			return ds[i].MAC < ds[j].MAC
		case errA != nil:
			return false
		case errB != nil:
			return true
		}
		return a.Less(b)
	})
}
