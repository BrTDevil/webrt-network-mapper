// Package wifi inventariază rețelele Wi-Fi vizibile folosind utilitarele sistemului.
package wifi

import (
	"context"
	"sort"
	"strings"
)

// Network e un BSSID văzut la scan.
type Network struct {
	SSID      string `json:"ssid"`
	BSSID     string `json:"bssid"`
	Channel   int    `json:"channel"`
	Frequency int    `json:"frequency"` // MHz
	Band      string `json:"band"`      // 2.4 | 5 | 6
	Signal    int    `json:"signal"`    // dBm
	Security  string `json:"security"`
}

// Scan rulează scanarea specifică sistemului de operare.
func Scan(ctx context.Context) ([]Network, error) {
	nets, err := scanOS(ctx)
	if err != nil {
		return nil, err
	}
	// Deduplicare după BSSID (păstrează semnalul cel mai bun).
	byB := map[string]Network{}
	for _, n := range nets {
		n.BSSID = normalizeBSSID(n.BSSID)
		if n.BSSID == "" {
			continue
		}
		if n.Frequency == 0 && n.Channel > 0 {
			n.Frequency = FreqFromChannel(n.Channel, n.Band)
		}
		if n.Channel == 0 && n.Frequency > 0 {
			n.Channel = ChannelFromFreq(n.Frequency)
		}
		if n.Band == "" {
			n.Band = BandOf(n.Channel, n.Frequency)
		}
		if old, ok := byB[n.BSSID]; !ok || n.Signal > old.Signal {
			byB[n.BSSID] = n
		}
	}
	out := make([]Network, 0, len(byB))
	for _, n := range byB {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SSID != out[j].SSID {
			return out[i].SSID < out[j].SSID
		}
		return out[i].Signal > out[j].Signal
	})
	return out, nil
}

// PercentToDBM aproximează dBm din procentul raportat (nmcli/netsh):
// 100% ≈ -50 dBm, 0% ≈ -100 dBm.
func PercentToDBM(p int) int {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p/2 - 100
}

// BandOf derivă banda din frecvență (preferat) sau canal.
func BandOf(channel, freq int) string {
	switch {
	case freq >= 5925:
		return "6"
	case freq >= 4900:
		return "5"
	case freq >= 2400:
		return "2.4"
	case channel >= 1 && channel <= 14:
		return "2.4"
	case channel >= 32 && channel <= 177:
		return "5"
	}
	return ""
}

// ChannelFromFreq convertește MHz în număr de canal.
func ChannelFromFreq(f int) int {
	switch {
	case f == 2484:
		return 14
	case f >= 2412 && f < 2484:
		return (f - 2407) / 5
	case f >= 5955 && f <= 7115:
		return (f - 5950) / 5
	case f >= 5000 && f < 5925:
		return (f - 5000) / 5
	}
	return 0
}

// FreqFromChannel e inversul lui ChannelFromFreq.
func FreqFromChannel(ch int, band string) int {
	switch {
	case band == "6":
		return 5950 + ch*5
	case ch == 14:
		return 2484
	case ch >= 1 && ch <= 13:
		return 2407 + ch*5
	case ch >= 32 && ch <= 177:
		return 5000 + ch*5
	}
	return 0
}

func normalizeBSSID(b string) string {
	b = strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(b, "-", ":")))
	parts := strings.Split(b, ":")
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}
