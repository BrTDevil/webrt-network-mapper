//go:build linux

package wifi

import "testing"

func TestParseNmcliEscaped(t *testing.T) {
	out := parseNmcli("RP:7C\\:F1\\:7E\\:7A\\:0A\\:34:6:2437 MHz:80:WPA2\n:7E\\:F1\\:7E\\:4A\\:0A\\:34:3:2422 MHz:58:WPA2\n")
	if len(out) != 2 || out[0].SSID != "RP" || out[0].BSSID != "7C:F1:7E:7A:0A:34" || out[0].Frequency != 2437 || out[0].Signal != -60 {
		t.Fatalf("parse greșit: %+v", out)
	}
}
