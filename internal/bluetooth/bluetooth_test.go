package bluetooth

import "testing"

func TestParseInfoAndClassify(t *testing.T) {
	info := []byte("Device 56:65:1D:19:5D:A3 (random)\n\tName: HOCO ES50\n\tAlias: HOCO ES50\n\tIcon: audio-headset\n\tPaired: no\n\tRSSI: 0xffffffb0 (-80)\n\tManufacturerData.Key: 0x004c (76)\n")
	var d Device
	parseInfo(info, &d)
	if d.Name != "HOCO ES50" || d.AddressType != "random" || d.Kind != "audio-headset" || d.RSSI == nil || *d.RSSI != -80 || d.Manufacturer != "Apple" {
		t.Fatalf("parse greșit: %+v", d)
	}
	if got := Classify(d); got != "casti" {
		t.Errorf("Classify = %s, vreau casti", got)
	}
	for name, want := range map[string]string{"BT5.4 Mouse": "mouse", "Amazfit Helio Strap": "ceas", "RPP02N": "imprimanta_bonuri", "DESKTOP-QAAOIPI": "laptop", "EVOLVEO": "boxa"} {
		if got := Classify(Device{Name: name}); got != want {
			t.Errorf("Classify(%q) = %s, vreau %s", name, got, want)
		}
	}
	if !looksLikeMAC("6B-2E-90-91-0D-20") {
		t.Error("alias MAC nerecunoscut")
	}
}
