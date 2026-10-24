// Package jobs rulează scanările și salvează rezultatele în baza de date.
// E folosit și de CLI, și de API.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"webrt-network-mapper/internal/bluetooth"
	"webrt-network-mapper/internal/db"
	"webrt-network-mapper/internal/scanner"
	"webrt-network-mapper/internal/wifi"
)

// Runner ține evidența scanărilor în curs (pentru anulare).
type Runner struct {
	DB      *db.DB
	mu      sync.Mutex
	running map[int64]context.CancelFunc
}

func NewRunner(d *db.DB) *Runner {
	return &Runner{DB: d, running: map[int64]context.CancelFunc{}}
}

// Request descrie ce se scanează.
type Request struct {
	LAN        bool   `json:"lan"`
	WiFi       bool   `json:"wifi"`
	Bluetooth  bool   `json:"bluetooth"`
	Interface  string `json:"interface"`
	Subnet     string `json:"subnet"`
	AllowLarge bool   `json:"allow_large"` // confirmare pentru subnet > /22
	// AllowOtherNetwork: scanează în acest proiect chiar dacă routerul rețelei curente
	// nu e unul văzut până acum aici (vezi CheckNetwork).
	AllowOtherNetwork bool `json:"allow_other_network"`
	// SSIDFilter: doar rețelele ale căror nume conțin unul din termeni (separați prin virgulă).
	SSIDFilter *string `json:"ssid_filter"`
	// ConnectSSID + Password: scanul LAN se face pe altă rețea Wi-Fi, cu parola dată
	// de utilizator. Parola nu se salvează nicăieri; după scan se revine la rețeaua inițială.
	ConnectSSID string `json:"connect_ssid"`
	Password    string `json:"password"`
}

// SSIDFilterKey e cheia de setări pentru filtrul SSID al unui proiect.
func SSIDFilterKey(projectID int64) string { return fmt.Sprintf("ssid_filter.%d", projectID) }

// MatchSSID: filtru gol = toate; altfel SSID-ul trebuie să conțină unul din termeni.
func MatchSSID(filter, ssid string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	s := strings.ToLower(ssid)
	for _, t := range strings.Split(filter, ",") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" && strings.Contains(s, t) {
			return true
		}
	}
	return false
}

// Start pornește scanările în fundal și întoarce ID-urile lor.
func (r *Runner) Start(projectID int64, req Request) ([]int64, error) {
	if !req.LAN && !req.WiFi && !req.Bluetooth {
		return nil, errors.New("alege cel puțin LAN, Wi-Fi sau Bluetooth")
	}
	if req.ConnectSSID != "" && !req.LAN {
		return nil, errors.New("conectarea la altă rețea are sens doar pentru scanul LAN")
	}
	// Cu ConnectSSID utilizatorul a ales explicit rețeaua, deci nu mai verificăm.
	if !req.AllowOtherNetwork && req.ConnectSSID == "" {
		if err := r.CheckNetwork(context.Background(), projectID, req.Interface); err != nil {
			return nil, err
		}
	}
	if req.SSIDFilter != nil {
		r.DB.SetSetting(SSIDFilterKey(projectID), strings.TrimSpace(*req.SSIDFilter))
	}
	var ids []int64
	var lanID, wifiID, btID int64
	var err error
	if req.LAN {
		if lanID, err = r.DB.CreateScan(projectID, "lan"); err != nil {
			return nil, err
		}
		ids = append(ids, lanID)
	}
	if req.WiFi {
		if wifiID, err = r.DB.CreateScan(projectID, "wifi"); err != nil {
			return nil, err
		}
		ids = append(ids, wifiID)
	}
	if req.Bluetooth {
		if btID, err = r.DB.CreateScan(projectID, "bluetooth"); err != nil {
			return nil, err
		}
		ids = append(ids, btID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	for _, id := range ids {
		r.running[id] = cancel
	}
	r.mu.Unlock()
	go func() {
		defer cancel()
		defer func() {
			r.mu.Lock()
			for _, id := range ids {
				delete(r.running, id)
			}
			r.mu.Unlock()
		}()
		// Bluetooth-ul ascultă în paralel (nu interferează cu scanul LAN).
		var wg sync.WaitGroup
		if btID != 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r.RunBluetooth(ctx, projectID, btID)
			}()
		}
		// LAN întâi: asocierea BSSID ↔ AP are nevoie de MAC-urile din LAN.
		if lanID != 0 {
			r.runLANMaybeConnected(ctx, projectID, lanID, req)
		}
		if wifiID != 0 {
			r.RunWiFi(ctx, projectID, wifiID, nil)
		}
		wg.Wait()
		// Legăturile Bluetooth ↔ LAN se refac după ce ambele scanări s-au terminat.
		r.linkBluetooth(projectID)
	}()
	return ids, nil
}

// ProjectRef identifică pe scurt un proiect (locație).
type ProjectRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// OtherNetworkError: laptopul e conectat la o rețea al cărei router nu a fost văzut
// niciodată în proiectul curent. Scanul ar amesteca inventarul a două locații.
type OtherNetworkError struct {
	Project    ProjectRef   `json:"project"`
	GatewayIP  string       `json:"gateway_ip"`
	GatewayMAC string       `json:"gateway_mac"`
	Matches    []ProjectRef `json:"matches"` // proiectele în care routerul a mai fost văzut
}

func (e *OtherNetworkError) Error() string {
	msg := fmt.Sprintf("rețeaua curentă (router %s, %s) nu a mai fost scanată în locația „%s”", e.GatewayIP, e.GatewayMAC, e.Project.Name)
	if len(e.Matches) > 0 {
		msg += fmt.Sprintf("; e rețeaua locației „%s”", e.Matches[0].Name)
	}
	return msg
}

// CheckNetwork întoarce *OtherNetworkError dacă proiectul are deja scanări LAN, dar
// routerul rețelei curente nu e printre routerele văzute în el. Un proiect nou (fără
// scanări) sau un router care nu poate fi identificat trec fără întrebări.
func (r *Runner) CheckNetwork(ctx context.Context, projectID int64, iface string) error {
	all, err := r.DB.ProjectGateways()
	if err != nil || len(all[projectID]) == 0 {
		return nil
	}
	gw, mac := scanner.GatewayMAC(ctx, iface)
	if mac == "" || slices.Contains(all[projectID], mac) {
		return nil
	}
	p, err := r.DB.GetProject(projectID)
	if err != nil {
		return err
	}
	e := &OtherNetworkError{Project: ProjectRef{p.ID, p.Name}, GatewayIP: gw, GatewayMAC: mac}
	for pid, macs := range all {
		if pid == projectID || !slices.Contains(macs, mac) {
			continue
		}
		if o, err := r.DB.GetProject(pid); err == nil {
			e.Matches = append(e.Matches, ProjectRef{o.ID, o.Name})
		}
	}
	slices.SortFunc(e.Matches, func(a, b ProjectRef) int { return int(a.ID - b.ID) })
	return e
}

// Cancel oprește o scanare în curs.
func (r *Runner) Cancel(scanID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.running[scanID]; ok {
		c()
		return true
	}
	return false
}

// RunLANConnected e varianta publică (CLI) a runLANMaybeConnected.
func (r *Runner) RunLANConnected(ctx context.Context, projectID, scanID int64, req Request) error {
	return r.runLANMaybeConnected(ctx, projectID, scanID, req)
}

// runLANMaybeConnected: dacă s-a cerut altă rețea, se conectează (cu parola dată
// de utilizator), scanează și revine la rețeaua de dinainte, orice s-ar întâmpla.
func (r *Runner) runLANMaybeConnected(ctx context.Context, projectID, scanID int64, req Request) error {
	if req.ConnectSSID == "" {
		return r.RunLAN(ctx, projectID, scanID, req)
	}
	r.DB.UpdateScanProgress(scanID, 1, fmt.Sprintf("Mă conectez la rețeaua %q…", req.ConnectSSID))
	sess, err := wifi.Connect(ctx, req.ConnectSSID, req.Password, req.Interface)
	req.Password = "" // nu mai avem nevoie de ea
	if err != nil {
		r.DB.FinishScan(scanID, "error", err.Error())
		return err
	}
	defer func() {
		r.DB.UpdateScanProgress(scanID, 99, "Revin la rețeaua inițială…")
		sess.Restore()
	}()
	req.Interface = sess.Interface
	req.Subnet = "" // subnetul noii rețele
	err = r.RunLAN(ctx, projectID, scanID, req)
	if err == nil {
		if sc, e := r.DB.GetScan(scanID); e == nil {
			r.DB.FinishScan(scanID, "done", sc.Message+fmt.Sprintf(" — rețeaua %q", req.ConnectSSID))
		}
	}
	return err
}

// RunLAN execută sincron un scan LAN deja creat în baza de date.
func (r *Runner) RunLAN(ctx context.Context, projectID, scanID int64, req Request) error {
	opt := scanner.Options{
		Interface: req.Interface,
		Subnet:    req.Subnet,
		Progress: func(p int, m string) {
			r.DB.UpdateScanProgress(scanID, p, m)
		},
	}
	if req.AllowLarge {
		opt.MaxHosts = 1 << 16
	}
	res, err := scanner.ScanLAN(ctx, opt)
	if err != nil {
		status := "error"
		if errors.Is(err, context.Canceled) {
			status = "cancelled"
		}
		var big *scanner.ErrSubnetTooLarge
		if errors.As(err, &big) {
			status = "needs_confirm"
		}
		r.DB.FinishScan(scanID, status, err.Error())
		return err
	}
	r.DB.UpdateScanInfo(scanID, res.Net.Prefix.String(), res.Net.Interface, res.Net.Gateway, strings.Join(res.Net.DNS, ", "), res.Method)
	ipsPerMAC := map[string]int{}
	for _, h := range res.Hosts {
		ipsPerMAC[h.MAC]++
	}
	for _, h := range res.Hosts {
		ports, _ := json.Marshal(h.Ports)
		svc := map[string]string{}
		for _, p := range h.Ports {
			svc[strconv.Itoa(p)] = scanner.ServiceNames[p]
		}
		svcJSON, _ := json.Marshal(svc)
		if _, err := r.DB.UpsertDevice(projectID, scanID, db.DeviceObservation{
			MAC: h.MAC, IP: h.IP.String(), Hostname: h.Hostname, Vendor: h.Vendor,
			DetectedType: h.Type, OSGuess: h.OS, OpenPorts: string(ports), Services: string(svcJSON), LatencyMS: h.LatencyMS,
			SharedMAC: ipsPerMAC[h.MAC] > 1,
		}); err != nil {
			r.DB.FinishScan(scanID, "error", err.Error())
			return err
		}
	}
	r.inferAPs(projectID)
	msg := fmt.Sprintf("%d dispozitive găsite (metodă: %s)", len(res.Hosts), res.Method)
	return r.DB.FinishScan(scanID, "done", msg)
}

// RunWiFi execută sincron un scan Wi-Fi. Cu surveyPointID, observațiile se leagă de punctul de survey.
func (r *Runner) RunWiFi(ctx context.Context, projectID, scanID int64, surveyPointID *int64) error {
	r.DB.UpdateScanProgress(scanID, 10, "Scanez rețelele Wi-Fi…")
	nets, err := wifi.Scan(ctx)
	if err != nil {
		r.DB.FinishScan(scanID, "error", err.Error())
		return err
	}
	filter := r.DB.GetSetting(SSIDFilterKey(projectID))
	kept := 0
	for _, n := range nets {
		if !MatchSSID(filter, n.SSID) {
			continue
		}
		kept++
		// BSSID-urile administrate local (SSID-uri secundare, oaspeți) nu au OUI real.
		vendor := ""
		if !scanner.IsRandomMAC(n.BSSID) {
			vendor = scanner.LookupVendor(n.BSSID)
		}
		if _, err := r.DB.UpsertWifi(projectID, scanID, surveyPointID, db.WifiObservation{
			SSID: n.SSID, BSSID: n.BSSID, Band: n.Band, Channel: n.Channel, Frequency: n.Frequency,
			Security: n.Security, Vendor: vendor, SignalDBM: n.Signal,
		}); err != nil {
			r.DB.FinishScan(scanID, "error", err.Error())
			return err
		}
	}
	r.inferAPs(projectID)
	msg := fmt.Sprintf("%d BSSID-uri găsite", kept)
	if filter != "" {
		msg += fmt.Sprintf(" (filtru „%s”; %d ignorate)", filter, len(nets)-kept)
	}
	return r.DB.FinishScan(scanID, "done", msg)
}

// RunBluetooth ascultă pasiv ~12 secunde anunțurile Bluetooth din jur.
func (r *Runner) RunBluetooth(ctx context.Context, projectID, scanID int64) error {
	r.DB.UpdateScanProgress(scanID, 5, "Ascult dispozitivele Bluetooth din apropiere (~12 s)…")
	devs, err := bluetooth.Scan(ctx, 12*time.Second)
	if err != nil {
		status := "error"
		if errors.Is(err, context.Canceled) {
			status = "cancelled"
		}
		r.DB.FinishScan(scanID, status, err.Error())
		return err
	}
	named := 0
	for _, d := range devs {
		if d.Name != "" {
			named++
		}
		if _, err := r.DB.UpsertBluetooth(projectID, scanID, db.BluetoothObservation{
			MAC: d.MAC, Name: d.Name, AddressType: d.AddressType, Kind: d.Kind, Manufacturer: d.Manufacturer,
			DetectedType: d.Type, Paired: d.Paired, RSSI: d.RSSI,
		}); err != nil {
			r.DB.FinishScan(scanID, "error", err.Error())
			return err
		}
	}
	r.linkBluetooth(projectID)
	return r.DB.FinishScan(scanID, "done", fmt.Sprintf("%d dispozitive Bluetooth (%d cu nume)", len(devs), named))
}

// linkBluetooth: un dispozitiv Bluetooth cu același nume ca unul din LAN
// (hostname sau nume dat de utilizator) e probabil același echipament.
func (r *Runner) linkBluetooth(projectID int64) {
	bts, err := r.DB.ListBluetooth(projectID)
	if err != nil {
		return
	}
	devs, err := r.DB.ListDevices(projectID)
	if err != nil {
		return
	}
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.TrimSuffix(s, ".local")
		if i := strings.IndexByte(s, '.'); i > 0 {
			s = s[:i]
		}
		return s
	}
	byName := map[string]int64{}
	for _, d := range devs {
		for _, n := range []string{d.Hostname, d.CustomName} {
			if k := norm(n); len(k) >= 3 {
				byName[k] = d.ID
			}
		}
	}
	for _, b := range bts {
		if b.LinkStatus == "confirmat" || b.LinkStatus == "respins" || b.Name == "" {
			continue
		}
		if id, ok := byName[norm(b.Name)]; ok {
			r.DB.SetInferredBTLink(b.ID, id)
		}
	}
}

// inferAPs propune asocieri BSSID ↔ dispozitiv LAN (status „inferat”).
func (r *Runner) inferAPs(projectID int64) {
	devs, err := r.DB.ListDevices(projectID)
	if err != nil {
		return
	}
	cands := make([]wifi.Candidate, 0, len(devs))
	for _, d := range devs {
		cands = append(cands, wifi.Candidate{ID: d.ID, MAC: d.MAC})
	}
	nets, err := r.DB.ListWifi(projectID)
	if err != nil {
		return
	}
	for _, n := range nets {
		if n.APLinkStatus == "confirmat" || n.APLinkStatus == "respins" {
			continue
		}
		if id := wifi.InferAP(n.BSSID, cands); id != 0 {
			r.DB.SetInferredAP(n.ID, id)
		}
	}
}
