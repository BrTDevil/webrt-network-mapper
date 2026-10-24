package wifi

// Current descrie rețeaua Wi-Fi la care e conectat laptopul.
type Current struct {
	SSID      string `json:"ssid"`
	BSSID     string `json:"bssid"`
	Interface string `json:"interface"`
}

// Session e o conexiune temporară făcută pentru scan; Restore o anulează.
type Session struct {
	Interface string
	prevUUID  string // conexiunea activă înainte (se reactivează la final)
	tempName  string // profil creat de noi (se șterge la final)
	switched  bool
}
