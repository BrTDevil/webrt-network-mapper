//go:build darwin

package wifi

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func scanOS(ctx context.Context) ([]Network, error) {
	out, err := exec.CommandContext(ctx, "system_profiler", "SPAirPortDataType", "-json").Output()
	if err != nil {
		return nil, err
	}
	return parseProfiler(out)
}

type spNet struct {
	Name     string `json:"_name"`
	BSSID    string `json:"spairport_network_bssid"`
	Channel  string `json:"spairport_network_channel"`
	Security string `json:"spairport_security_mode"`
	SigNoise string `json:"spairport_signal_noise"`
}

func parseProfiler(b []byte) ([]Network, error) {
	var doc struct {
		Data []struct {
			Interfaces []struct {
				Current *spNet  `json:"spairport_current_network_information"`
				Others  []spNet `json:"spairport_airport_other_local_wireless_networks"`
			} `json:"spairport_airport_interfaces"`
		} `json:"SPAirPortDataType"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	reNum := regexp.MustCompile(`-?\d+`)
	var out []Network
	add := func(n spNet) {
		// Canal ex. „36 (5GHz, 80MHz)” sau „6 (2GHz, 20MHz)”.
		ch, _ := strconv.Atoi(reNum.FindString(n.Channel))
		band := ""
		switch {
		case strings.Contains(n.Channel, "6GHz"):
			band = "6"
		case strings.Contains(n.Channel, "5GHz"):
			band = "5"
		case strings.Contains(n.Channel, "2GHz"):
			band = "2.4"
		}
		sig, _ := strconv.Atoi(reNum.FindString(n.SigNoise))
		sec := strings.TrimPrefix(n.Security, "spairport_security_mode_")
		// macOS recent ascunde BSSID-ul fără permisiune de localizare;
		// folosim un identificator sintetic stabil ca să nu pierdem rețeaua.
		bssid := n.BSSID
		if bssid == "" {
			bssid = syntheticBSSID(n.Name, ch)
		}
		out = append(out, Network{SSID: n.Name, BSSID: bssid, Channel: ch, Band: band, Signal: sig, Security: sec})
	}
	for _, d := range doc.Data {
		for _, it := range d.Interfaces {
			if it.Current != nil {
				add(*it.Current)
			}
			for _, o := range it.Others {
				add(o)
			}
		}
	}
	return out, nil
}

func syntheticBSSID(ssid string, ch int) string {
	var h uint32 = 2166136261
	for _, c := range []byte(ssid + ":" + strconv.Itoa(ch)) {
		h ^= uint32(c)
		h *= 16777619
	}
	// 02:… = adresă administrată local, deci nu se confundă cu una reală.
	return strings.ToUpper(strings.Join([]string{"02", "00", hex2(byte(h >> 24)), hex2(byte(h >> 16)), hex2(byte(h >> 8)), hex2(byte(h))}, ":"))
}

func hex2(b byte) string {
	const d = "0123456789abcdef"
	return string([]byte{d[b>>4], d[b&15]})
}
