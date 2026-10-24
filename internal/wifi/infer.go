package wifi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// MACToUint întoarce MAC-ul ca număr pe 48 de biți.
func MACToUint(mac string) (uint64, bool) {
	s := strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", "")
	if len(s) != 12 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 64)
	return v, err == nil
}

// SameRadioPair: două BSSID-uri care diferă doar în ultimii 4 biți
// (sau doar pe ultimul octet cu același prefix de 5 octeți) sunt probabil
// radiourile aceluiași echipament.
func SameRadioPair(a, b string) bool {
	x, ok1 := MACToUint(a)
	y, ok2 := MACToUint(b)
	if !ok1 || !ok2 || x == y {
		return false
	}
	// Ignoră bitul „locally administered” pe care unii producători îl setează pe BSSID-uri secundare.
	const la = uint64(0x02) << 40
	x &^= la
	y &^= la
	return x>>8 == y>>8 || x>>4 == y>>4
}

// Candidate e un dispozitiv LAN comparat cu BSSID-urile.
type Candidate struct {
	ID  int64
	MAC string
}

// InferAP caută dispozitivul LAN al cărui MAC e apropiat numeric de BSSID
// (același OUI, diferență ≤ 8 pe ultimii octeți). Întoarce ID-ul sau 0.
func InferAP(bssid string, devices []Candidate) int64 {
	b, ok := MACToUint(bssid)
	if !ok {
		return 0
	}
	const la = uint64(0x02) << 40
	bssidLA := b&la != 0
	b &^= la
	best, bestDiff := int64(0), uint64(9)
	for _, d := range devices {
		m, ok := MACToUint(d.MAC)
		if !ok {
			continue
		}
		m &^= la
		var diff uint64
		switch {
		case m>>24 == b>>24: // același OUI: compară toată adresa
			diff = absDiff(m, b)
		case bssidLA:
			// BSSID administrat local (ex. TP-Link: 7A:06:C3:… pentru routerul C0:06:C3:…):
			// producătorul modifică primul octet, partea NIC rămâne apropiată.
			if (m>>24)&0xFFFF != (b>>24)&0xFFFF {
				continue
			}
			diff = absDiff(m&0xFFFFFF, b&0xFFFFFF)
		default:
			continue
		}
		if diff < bestDiff {
			best, bestDiff = d.ID, diff
		}
	}
	return best
}

// Warning e o observație automată pentru raport.
type Warning struct {
	Level   string `json:"level"` // info | warn
	Message string `json:"message"`
}

// AnalyzeInput e setul minim de date pentru analiză.
type AnalyzeInput struct {
	SSID    string
	BSSID   string
	Band    string
	Channel int
	Signal  *int
}

// Analyze detectează canale suprapuse pe 2.4 GHz și AP-uri puternice pe același canal.
func Analyze(nets []AnalyzeInput) []Warning {
	var ws []Warning
	sameChan := map[string][]AnalyzeInput{}
	for _, n := range nets {
		if n.Band == "2.4" && n.Channel != 0 && n.Channel != 1 && n.Channel != 6 && n.Channel != 11 {
			ws = append(ws, Warning{"warn", fmt.Sprintf("%s (%s) folosește canalul %d pe 2.4 GHz — se suprapune cu canalele vecine; recomandat 1, 6 sau 11.", nameOr(n.SSID), n.BSSID, n.Channel)})
		}
		if n.Signal != nil && *n.Signal >= -70 && n.Channel > 0 {
			k := n.Band + "/" + strconv.Itoa(n.Channel)
			sameChan[k] = append(sameChan[k], n)
		}
	}
	keys := make([]string, 0, len(sameChan))
	for k := range sameChan {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := sameChan[k]
		// Radiourile aceluiași echipament nu contează ca AP-uri diferite.
		distinct := []AnalyzeInput{}
		for _, n := range g {
			dup := false
			for _, d := range distinct {
				if SameRadioPair(n.BSSID, d.BSSID) {
					dup = true
				}
			}
			if !dup {
				distinct = append(distinct, n)
			}
		}
		if len(distinct) >= 2 {
			names := []string{}
			for _, n := range distinct {
				names = append(names, fmt.Sprintf("%s (%s, %d dBm)", nameOr(n.SSID), n.BSSID, *n.Signal))
			}
			parts := strings.SplitN(k, "/", 2)
			ws = append(ws, Warning{"warn", fmt.Sprintf("%d AP-uri cu semnal puternic pe același canal %s (%s GHz): %s. Interferență probabilă — mută-le pe canale diferite.", len(distinct), parts[1], parts[0], strings.Join(names, "; "))})
		}
	}
	return ws
}

func nameOr(s string) string {
	if s == "" {
		return "<SSID ascuns>"
	}
	return s
}

func absDiff(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}
