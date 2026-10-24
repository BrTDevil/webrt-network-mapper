package export

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"webrt-network-mapper/internal/db"
)

// DisplayName întoarce numele dat de utilizator, apoi hostname-ul, apoi IP-ul.
func DisplayName(d db.Device) string {
	switch {
	case d.CustomName != "":
		return d.CustomName
	case d.Hostname != "":
		return d.Hostname
	case d.LastIP != "":
		return d.LastIP
	}
	return d.MAC
}

// EffectiveType: tipul ales de utilizator suprascrie tipul detectat.
func EffectiveType(d db.Device) string {
	if d.DeviceType != "" {
		return d.DeviceType
	}
	if d.DetectedType != "" {
		return d.DetectedType
	}
	return "necunoscut"
}

// PortsString transformă JSON-ul „[22,80]” în „22, 80”.
func PortsString(js string) string {
	var ports []int
	json.Unmarshal([]byte(js), &ports)
	s := make([]string, len(ports))
	for i, p := range ports {
		s[i] = strconv.Itoa(p)
	}
	return strings.Join(s, ", ")
}

// WriteCSV scrie inventarul de dispozitive.
func WriteCSV(w io.Writer, devices []db.Device) error {
	// BOM ca Excel să recunoască UTF-8 (diacritice).
	w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	cw.Write([]string{"name", "type", "ip", "mac", "vendor", "hostname", "os", "ports", "location", "notes", "last_seen"})
	for _, d := range devices {
		cw.Write([]string{DisplayName(d), EffectiveType(d), d.LastIP, d.MAC, d.Vendor, d.Hostname, d.OSGuess,
			PortsString(d.OpenPorts), d.Location, d.Notes, d.LastSeen})
	}
	cw.Flush()
	return cw.Error()
}
