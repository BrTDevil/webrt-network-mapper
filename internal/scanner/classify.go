package scanner

import (
	"strings"
)

// Tipuri de dispozitive (aceleași chei ca icon-urile din frontend).
const (
	TypeRouter  = "router"
	TypeAP      = "ap"
	TypeSwitch  = "switch"
	TypePrinter = "imprimanta"
	TypeCamera  = "camera"
	TypePC      = "pc"
	TypeLaptop  = "laptop"
	TypeServer  = "server"
	TypeNAS     = "nas"
	TypePhone   = "telefon"
	TypeTablet  = "tableta"
	TypeTV      = "tv"
	TypeUnknown = "necunoscut"
	TypeVoIP    = "telefon_ip"
	TypePOS     = "terminal_pos"
	TypeNVR     = "nvr"
	TypeSelf    = TypeLaptop // calculatorul pe care rulează scanul
)

// ClassifyInput conține ce știm despre un host.
type ClassifyInput struct {
	IP, MAC, Vendor, Hostname string
	Ports                     []int
	IsGateway, IsSelf         bool
}

// Classify ghicește tipul dispozitivului și, unde se poate, sistemul de operare.
func Classify(in ClassifyInput) (typ, os string) {
	has := map[int]bool{}
	for _, p := range in.Ports {
		has[p] = true
	}
	v := strings.ToLower(in.Vendor)
	h := strings.ToLower(in.Hostname)
	vendorIs := func(names ...string) bool {
		for _, n := range names {
			if strings.Contains(v, n) {
				return true
			}
		}
		return false
	}
	hostIs := func(names ...string) bool {
		for _, n := range names {
			if strings.Contains(h, n) {
				return true
			}
		}
		return false
	}

	switch {
	case in.IsSelf:
		return TypeSelf, ""
	case in.IsGateway:
		return TypeRouter, ""
	case has[9100] || has[515] || has[631] || hostIs("printer", "imprimanta", "epson", "brother", "hp-", "canon") ||
		vendorIs("epson", "brother", "zebra", "star micronics", "bixolon", "citizen"):
		return TypePrinter, ""
	case has[554] || vendorIs("hikvision", "dahua", "axis comm", "uniview", "reolink", "ezviz"):
		if vendorIs("hikvision", "dahua") && (has[8000] || !has[554]) && hostIs("nvr", "dvr") {
			return TypeNVR, ""
		}
		return TypeCamera, ""
	case vendorIs("synology", "qnap", "western digital") || hostIs("nas", "diskstation"):
		return TypeNAS, "Linux"
	case vendorIs("ubiquiti", "tp-link", "mikrotik", "routerboard", "netgear", "d-link", "zyxel", "tenda", "mercusys", "ruckus", "aruba", "cambium", "edimax", "asustek") &&
		(has[80] || has[443] || has[8080] || has[22]) && !has[3389] && !has[445]:
		if has[53] {
			return TypeRouter, ""
		}
		return TypeAP, ""
	case vendorIs("cisco", "hewlett packard enterprise", "juniper") && (has[22] || has[23]) && !has[3389]:
		return TypeSwitch, ""
	case vendorIs("yealink", "grandstream", "snom", "polycom", "fanvil", "gigaset"):
		return TypeVoIP, ""
	case vendorIs("ingenico", "verifone", "pax technology", "newland"):
		return TypePOS, ""
	case has[3389] || (has[445] && has[139]):
		return TypePC, "Windows"
	case has[22] && !has[445] && (has[80] || has[443]) && !vendorIs("apple", "samsung", "xiaomi"):
		return TypeServer, "Linux"
	case vendorIs("samsung", "lg electronics", "sony", "philips", "tcl", "hisense", "roku") && (has[8080] || has[8443] || hostIs("tv")):
		return TypeTV, ""
	case hostIs("ipad", "tab"):
		return TypeTablet, osFromVendor(v)
	case hostIs("iphone", "android", "galaxy", "redmi", "pixel", "phone"):
		return TypePhone, osFromVendor(v)
	case len(in.Ports) == 0 && (vendorIs("apple", "samsung", "xiaomi", "huawei", "oneplus", "oppo", "motorola", "google", "realme", "vivo") || IsRandomMAC(in.MAC)):
		return TypePhone, osFromVendor(v)
	case hostIs("macbook", "laptop", "thinkpad"):
		return TypeLaptop, osFromVendor(v)
	case vendorIs("intel", "dell", "lenovo", "hewlett", "micro-star", "asrock", "gigabyte", "realtek") || hostIs("desktop-", "pc"):
		return TypePC, ""
	case vendorIs("espressif", "tuya", "shelly", "sonoff", "raspberry"):
		return TypeUnknown, "IoT"
	}
	return TypeUnknown, ""
}

func osFromVendor(v string) string {
	switch {
	case strings.Contains(v, "apple"):
		return "iOS/macOS"
	case v == "":
		return ""
	default:
		return "Android"
	}
}
