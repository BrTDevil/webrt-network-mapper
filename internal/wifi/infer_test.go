package wifi

import "testing"

func TestInferAP(t *testing.T) {
	devs := []Candidate{{ID: 1, MAC: "C0:06:C3:CB:77:C8"}, {ID: 2, MAC: "68:54:5A:8E:6C:C8"}}
	cases := map[string]int64{
		"7A:06:C3:CB:77:C0": 1, // TP-Link, BSSID administrat local
		"C0:06:C3:CB:77:C9": 1, // același OUI, +1
		"C0:06:C3:CB:78:C8": 0, // prea departe
		"40:9B:CD:A3:B9:14": 0,
	}
	for bssid, want := range cases {
		if got := InferAP(bssid, devs); got != want {
			t.Errorf("InferAP(%s) = %d, vreau %d", bssid, got, want)
		}
	}
}

func TestSameRadioPair(t *testing.T) {
	if !SameRadioPair("7C:F1:7E:7A:0A:34", "7C:F1:7E:7A:0A:36") {
		t.Error("pereche 2.4/5 GHz nerecunoscută")
	}
	if SameRadioPair("7C:F1:7E:7A:0A:34", "40:9B:CD:A3:B9:14") {
		t.Error("fals pozitiv")
	}
}

func TestBand(t *testing.T) {
	if BandOf(0, 5180) != "5" || BandOf(0, 2412) != "2.4" || BandOf(0, 5975) != "6" || BandOf(149, 0) != "5" {
		t.Error("bandă greșită")
	}
	if ChannelFromFreq(5180) != 36 || ChannelFromFreq(2437) != 6 {
		t.Error("canal greșit")
	}
}
