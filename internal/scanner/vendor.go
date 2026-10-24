package scanner

import (
	"bytes"
	"encoding/csv"
	"strings"
	"sync"

	"webrt-network-mapper/assets"
)

var (
	ouiOnce sync.Once
	ouiMap  map[string]string
)

func loadOUI() {
	ouiMap = map[string]string{}
	r := csv.NewReader(bytes.NewReader(assets.OUI))
	recs, err := r.ReadAll()
	if err != nil {
		return
	}
	for _, rec := range recs[1:] {
		if len(rec) >= 2 {
			ouiMap[rec[0]] = rec[1]
		}
	}
}

// LookupVendor întoarce producătorul după primii 3 octeți ai MAC-ului.
func LookupVendor(mac string) string {
	mac = normalizeMAC(mac)
	if len(mac) < 8 {
		return ""
	}
	ouiOnce.Do(loadOUI)
	key := strings.ReplaceAll(mac[:8], ":", "")
	if v, ok := ouiMap[key]; ok {
		return shortVendor(v)
	}
	if IsRandomMAC(mac) {
		return "MAC privat (aleator)"
	}
	return ""
}

// IsRandomMAC: bitul „locally administered” setat — tipic pentru telefoane
// cu MAC privat per rețea.
func IsRandomMAC(mac string) bool {
	mac = normalizeMAC(mac)
	if len(mac) < 2 {
		return false
	}
	var b byte
	for _, c := range mac[:2] {
		b <<= 4
		switch {
		case c >= '0' && c <= '9':
			b |= byte(c - '0')
		case c >= 'A' && c <= 'F':
			b |= byte(c-'A') + 10
		}
	}
	return b&0x02 != 0
}

// shortVendor scurtează denumirile lungi („Apple, Inc.” → „Apple”).
func shortVendor(v string) string {
	v = strings.TrimSpace(v)
	for _, suf := range []string{",", " Co.", " Corporation", " Inc", " Ltd", " LTD", " Limited", " Technologies", " Technology", " GmbH", " Corp", " S.A.", " AG"} {
		if i := strings.Index(v, suf); i > 2 {
			v = v[:i]
		}
	}
	return strings.TrimSpace(v)
}

// normalizeMAC aduce MAC-ul la forma AA:BB:CC:DD:EE:FF.
func normalizeMAC(mac string) string {
	mac = strings.ToUpper(strings.TrimSpace(mac))
	mac = strings.ReplaceAll(mac, "-", ":")
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return mac
	}
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}

// NormalizeMAC e varianta exportată.
func NormalizeMAC(mac string) string { return normalizeMAC(mac) }
