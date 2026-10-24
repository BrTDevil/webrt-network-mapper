// Package bluetooth descoperă pasiv dispozitivele Bluetooth din apropiere.
//
// Doar ascultă anunțurile (advertising / inquiry) pe care dispozitivele le emit
// singure: nu împerechează, nu se conectează, nu încearcă PIN-uri.
package bluetooth

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// Device e un dispozitiv Bluetooth văzut.
type Device struct {
	MAC          string `json:"mac"`
	Name         string `json:"name"`         // gol dacă dispozitivul nu își anunță numele
	AddressType  string `json:"address_type"` // public | random
	Kind         string `json:"kind"`         // ex. phone, audio-headset, input-mouse
	Manufacturer string `json:"manufacturer"`
	Paired       bool   `json:"paired"`
	RSSI         *int   `json:"rssi"`
	Type         string `json:"type"` // cheie de icon ghicită
}

// Scan ascultă dur secunde și întoarce dispozitivele văzute.
func Scan(ctx context.Context, dur time.Duration) ([]Device, error) {
	devs, err := scanOS(ctx, dur)
	if err != nil {
		return nil, err
	}
	for i := range devs {
		d := &devs[i]
		d.MAC = strings.ToUpper(d.MAC)
		if looksLikeMAC(d.Name) {
			d.Name = "" // BlueZ pune MAC-ul ca alias când nu există nume
		}
		d.Type = Classify(*d)
	}
	return devs, nil
}

var reMacName = regexp.MustCompile(`^([0-9A-Fa-f]{2}[-:]){5}[0-9A-Fa-f]{2}$`)

func looksLikeMAC(s string) bool { return reMacName.MatchString(strings.TrimSpace(s)) }

// Companii Bluetooth SIG frecvente (ManufacturerData.Key).
var companies = map[int]string{
	0x0002: "Intel", 0x0006: "Microsoft", 0x000F: "Broadcom", 0x001D: "Qualcomm", 0x004C: "Apple",
	0x0059: "Nordic Semiconductor", 0x0075: "Samsung", 0x0087: "Garmin", 0x00E0: "Google", 0x0101: "Fitbit",
	0x010F: "HiSilicon / Huawei", 0x0131: "Cypress", 0x0157: "Huami (Amazfit)", 0x0171: "Amazon", 0x027D: "Huawei",
	0x02E5: "Espressif", 0x038F: "Xiaomi", 0x0499: "Ruuvi", 0x05A7: "Sonos", 0x0822: "Oppo", 0x0078: "Nike",
	0x00D2: "Dialog / Renesas", 0x0310: "SGL Italia", 0x0046: "MediaTek", 0x0A12: "Zebra", 0x0030: "ST Microelectronics",
	0x0003: "IBM", 0x000D: "Texas Instruments", 0x0056: "Sony Ericsson", 0x012D: "Sony", 0x00C4: "LG Electronics",
	0x00C7: "Quintic", 0x0110: "Nippon Seiki", 0x0969: "Xiaomi", 0x0BDA: "Realtek",
}

// CompanyName întoarce producătorul după ID-ul Bluetooth SIG.
func CompanyName(id int) string { return companies[id] }

// Classify ghicește tipul (cheile sunt aceleași ca icon-urile din frontend).
func Classify(d Device) string {
	n := strings.ToLower(d.Name)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(n, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("printer", "print", "rpp", "mpop", "zebra", "bixolon", "star ", "sm-l", "tm-p", "xprinter", "pos-", "pt-2", "goojprt", "munbyn"):
		return "imprimanta_bonuri"
	case has("pax", "ingenico", "verifone", "sumup", "sunmi", "move/5000"):
		return "terminal_pos"
	case has("scanner", "barcode", "honeywell", "datalogic", "symbol"):
		return "scanner_coduri"
	case has("mouse"), d.Kind == "input-mouse":
		return "mouse"
	case has("keyboard", "tastatura"), d.Kind == "input-keyboard":
		return "mouse"
	case has("buds", "airpods", "headset", "headphone", "earbud", "wh-", "wf-", "es50", "jbl tune", "freebuds", "căști", "casti"),
		d.Kind == "audio-headset" || d.Kind == "audio-headphones":
		return "casti"
	case has("speaker", "boxa", "jbl", "soundbar", "sonos", "bose", "flip", "charge", "evolveo", "marshall"), d.Kind == "audio-card":
		return "boxa"
	case has("watch", "band", "strap", "amazfit", "fitbit", "garmin", "mi smart", "helio", "ceas"):
		return "ceas"
	case has("tv", "bravia", "[tv]", "chromecast", "fire tv", "roku"):
		return "tv"
	case d.Kind == "phone" || has("iphone", "galaxy", "redmi", "pixel", "phone", "android"):
		return "telefon"
	case has("ipad", "tab", "tablet"):
		return "tableta"
	case d.Kind == "computer" || has("desktop-", "laptop", "macbook", "-pc", "pc-", "thinkpad"):
		return "laptop"
	case d.Kind == "printer":
		return "imprimanta"
	case d.Kind == "camera-video" || d.Kind == "camera-photo":
		return "camera"
	}
	return "bluetooth"
}
