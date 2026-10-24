package db

import (
	"database/sql"
	"errors"
	"strings"
)

// ErrNotFound e returnat când o înregistrare lipsește.
var ErrNotFound = errors.New("nu există")

// ErrVersionConflict e returnat la salvarea unei hărți modificate între timp.
var ErrVersionConflict = errors.New("harta a fost modificată între timp")

type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Scan struct {
	ID         int64  `json:"id"`
	ProjectID  int64  `json:"project_id"`
	Type       string `json:"type"`
	Subnet     string `json:"subnet"`
	Interface  string `json:"interface"`
	Gateway    string `json:"gateway"`
	DNS        string `json:"dns"`
	Method     string `json:"method"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"`
	Message    string `json:"message"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

type Device struct {
	ID           int64   `json:"id"`
	ProjectID    int64   `json:"project_id"`
	MAC          string  `json:"mac"`
	IPKey        string  `json:"ip_key,omitempty"` // IP-ul, când MAC-ul e partajat de mai multe IP-uri
	LastIP       string  `json:"last_ip"`
	Hostname     string  `json:"hostname"`
	Vendor       string  `json:"vendor"`
	DetectedType string  `json:"detected_type"`
	OSGuess      string  `json:"os_guess"`
	CustomName   string  `json:"custom_name"`
	DeviceType   string  `json:"device_type"`
	Location     string  `json:"location"`
	Notes        string  `json:"notes"`
	Color        string  `json:"color"`
	FirstSeen    string  `json:"first_seen"`
	LastSeen     string  `json:"last_seen"`
	OpenPorts    string  `json:"open_ports"` // JSON, din ultima observație
	Services     string  `json:"services"`
	LatencyMS    float64 `json:"latency_ms"`
	Online       bool    `json:"online"` // văzut la ultimul scan LAN
	New          bool    `json:"new"`    // apărut prima dată la ultimul scan LAN
}

type WifiNetwork struct {
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"project_id"`
	SSID         string `json:"ssid"`
	BSSID        string `json:"bssid"`
	Band         string `json:"band"`
	Channel      int    `json:"channel"`
	Frequency    int    `json:"frequency"`
	Security     string `json:"security"`
	Vendor       string `json:"vendor"`
	APDeviceID   *int64 `json:"ap_device_id"`
	APLinkStatus string `json:"ap_link_status"`
	Label        string `json:"label"`
	Location     string `json:"location"`
	Notes        string `json:"notes"`
	FirstSeen    string `json:"first_seen"`
	LastSeen     string `json:"last_seen"`
	SignalDBM    *int   `json:"signal_dbm"` // din ultima observație
	Visible      bool   `json:"visible"`    // văzut la ultimul scan Wi-Fi
}

type SurveyPoint struct {
	ID        int64           `json:"id"`
	ProjectID int64           `json:"project_id"`
	MapID     *int64          `json:"map_id"`
	Label     string          `json:"label"`
	X         float64         `json:"x"`
	Y         float64         `json:"y"`
	CreatedAt string          `json:"created_at"`
	Readings  []SurveyReading `json:"readings"`
}

type SurveyReading struct {
	WifiID    int64  `json:"wifi_id"`
	SSID      string `json:"ssid"`
	BSSID     string `json:"bssid"`
	Band      string `json:"band"`
	Channel   int    `json:"channel"`
	SignalDBM int    `json:"signal_dbm"`
}

type Map struct {
	ID         int64  `json:"id"`
	ProjectID  int64  `json:"project_id"`
	Name       string `json:"name"`
	SceneJSON  string `json:"scene_json"`
	Background string `json:"background"`
	Version    int    `json:"version"`
	UpdatedAt  string `json:"updated_at"`
}

// ns transformă NULL în șir gol.
func ns(s sql.NullString) string { return s.String }

// ---------- Proiecte ----------

func (d *DB) ListProjects() ([]Project, error) {
	rows, err := d.Query(`SELECT id, name, COALESCE(address,''), COALESCE(notes,''), created_at, updated_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Address, &p.Notes, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) GetProject(id int64) (*Project, error) {
	var p Project
	err := d.QueryRow(`SELECT id, name, COALESCE(address,''), COALESCE(notes,''), created_at, updated_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Address, &p.Notes, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (d *DB) CreateProject(name, address, notes string) (*Project, error) {
	now := Now()
	res, err := d.Exec(`INSERT INTO projects(name, address, notes, created_at, updated_at) VALUES (?,?,?,?,?)`, name, address, notes, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	// Fiecare proiect pornește cu o hartă goală.
	if _, err := d.CreateMap(id, "Parter", DefaultScene); err != nil {
		return nil, err
	}
	return d.GetProject(id)
}

// ProjectGateways întoarce, pentru fiecare proiect, MAC-urile routerelor văzute la
// scanările LAN terminate: dispozitivul care avea IP-ul gateway-ului în acel scan.
func (d *DB) ProjectGateways() (map[int64][]string, error) {
	rows, err := d.Query(`SELECT DISTINCT s.project_id, dv.mac
		FROM scans s
		JOIN device_observations o ON o.scan_id = s.id
		JOIN devices dv ON dv.id = o.device_id
		WHERE s.type = 'lan' AND s.status = 'done' AND COALESCE(s.gateway, '') <> '' AND o.ip = s.gateway`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]string{}
	for rows.Next() {
		var pid int64
		var mac string
		if err := rows.Scan(&pid, &mac); err != nil {
			return nil, err
		}
		out[pid] = append(out[pid], mac)
	}
	return out, rows.Err()
}

func (d *DB) UpdateProject(id int64, name, address, notes string) error {
	res, err := d.Exec(`UPDATE projects SET name=?, address=?, notes=?, updated_at=? WHERE id=?`, name, address, notes, Now(), id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// DeleteProject șterge proiectul și toate datele lui.
func (d *DB) DeleteProject(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`DELETE FROM wifi_observations WHERE scan_id IN (SELECT id FROM scans WHERE project_id = ?)`,
		`DELETE FROM bluetooth_observations WHERE scan_id IN (SELECT id FROM scans WHERE project_id = ?)`,
		`DELETE FROM bluetooth_devices WHERE project_id = ?`,
		`DELETE FROM device_observations WHERE scan_id IN (SELECT id FROM scans WHERE project_id = ?)`,
		`DELETE FROM wifi_networks WHERE project_id = ?`,
		`DELETE FROM survey_points WHERE project_id = ?`,
		`DELETE FROM devices WHERE project_id = ?`,
		`DELETE FROM scans WHERE project_id = ?`,
		`DELETE FROM maps WHERE project_id = ?`,
		`DELETE FROM projects WHERE id = ?`,
		// id-ul poate fi refolosit de o locație nouă: nu lăsăm setări moștenite
		`DELETE FROM settings WHERE key = 'ssid_filter.' || ?`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- Scanări ----------

func (d *DB) CreateScan(projectID int64, typ string) (int64, error) {
	res, err := d.Exec(`INSERT INTO scans(project_id, type, status, progress, started_at) VALUES (?,?, 'running', 0, ?)`, projectID, typ, Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateScanInfo(id int64, subnet, iface, gateway, dns, method string) error {
	_, err := d.Exec(`UPDATE scans SET subnet=?, interface=?, gateway=?, dns=?, method=? WHERE id=?`, subnet, iface, gateway, dns, method, id)
	return err
}

func (d *DB) UpdateScanProgress(id int64, progress int, message string) error {
	_, err := d.Exec(`UPDATE scans SET progress=?, message=? WHERE id=?`, progress, message, id)
	return err
}

func (d *DB) FinishScan(id int64, status, message string) error {
	_, err := d.Exec(`UPDATE scans SET status=?, message=?, progress=100, finished_at=? WHERE id=?`, status, message, Now(), id)
	return err
}

const scanCols = `id, project_id, type, COALESCE(subnet,''), COALESCE(interface,''), COALESCE(gateway,''), COALESCE(dns,''),
	COALESCE(method,''), status, progress, COALESCE(message,''), started_at, COALESCE(finished_at,'')`

func scanScan(sc interface{ Scan(...any) error }) (*Scan, error) {
	var s Scan
	err := sc.Scan(&s.ID, &s.ProjectID, &s.Type, &s.Subnet, &s.Interface, &s.Gateway, &s.DNS, &s.Method, &s.Status, &s.Progress, &s.Message, &s.StartedAt, &s.FinishedAt)
	return &s, err
}

func (d *DB) GetScan(id int64) (*Scan, error) {
	s, err := scanScan(d.QueryRow(`SELECT `+scanCols+` FROM scans WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

// ListScans întoarce scanările unui proiect, cele mai noi primele.
func (d *DB) ListScans(projectID int64) ([]Scan, error) {
	rows, err := d.Query(`SELECT `+scanCols+` FROM scans WHERE project_id = ? ORDER BY id DESC LIMIT 50`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Scan{}
	for rows.Next() {
		s, err := scanScan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// LastLANScan întoarce ultimul scan LAN terminat cu succes (sau nil).
func (d *DB) LastLANScan(projectID int64) (*Scan, error) {
	s, err := scanScan(d.QueryRow(`SELECT `+scanCols+` FROM scans WHERE project_id = ? AND type = 'lan' AND status = 'done' ORDER BY id DESC LIMIT 1`, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func (d *DB) lastScanID(projectID int64, typ string) int64 {
	var id int64
	d.QueryRow(`SELECT id FROM scans WHERE project_id = ? AND type = ? AND status = 'done' ORDER BY id DESC LIMIT 1`, projectID, typ).Scan(&id)
	return id
}

// ---------- Dispozitive ----------

// DeviceObservation e rezultatul scanării unui host.
type DeviceObservation struct {
	MAC          string
	IP           string
	Hostname     string
	Vendor       string
	DetectedType string
	OSGuess      string
	OpenPorts    string
	Services     string
	LatencyMS    float64
	// SharedMAC: în același scan, MAC-ul a răspuns și pentru alte IP-uri.
	SharedMAC bool
}

// deviceIPKey alege identitatea dispozitivului: normal doar MAC-ul (ip_key = ”),
// dar dacă MAC-ul e partajat, fiecare IP e un dispozitiv separat (ip_key = IP).
// Înregistrările existente sunt refolosite ca să nu se piardă numele date de utilizator.
func (d *DB) deviceIPKey(projectID int64, o DeviceObservation) (string, error) {
	var n int
	if !o.SharedMAC {
		// MAC-ul nu (mai) e partajat: rămânem pe înregistrarea după MAC, dacă există,
		// altfel pe cea după IP creată la un scan anterior.
		if err := d.QueryRow(`SELECT COUNT(*) FROM devices WHERE project_id = ? AND mac = ? AND ip_key = ''`, projectID, o.MAC).Scan(&n); err != nil || n > 0 {
			return "", err
		}
		if err := d.QueryRow(`SELECT COUNT(*) FROM devices WHERE project_id = ? AND mac = ? AND ip_key = ?`, projectID, o.MAC, o.IP).Scan(&n); err != nil || n > 0 {
			return o.IP, err
		}
		return "", nil
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM devices WHERE project_id = ? AND mac = ? AND ip_key = ?`, projectID, o.MAC, o.IP).Scan(&n); err != nil || n > 0 {
		return o.IP, err
	}
	// Prima dată când MAC-ul apare partajat: înregistrarea veche (după MAC) rămâne
	// dispozitivul de pe IP-ul pe care îl avea, restul IP-urilor devin dispozitive noi.
	_, err := d.Exec(`UPDATE devices SET ip_key = ? WHERE project_id = ? AND mac = ? AND ip_key = '' AND last_ip = ?`,
		o.IP, projectID, o.MAC, o.IP)
	return o.IP, err
}

// UpsertDevice inserează sau actualizează un dispozitiv după MAC (și IP, dacă
// MAC-ul e partajat). Nu atinge niciodată câmpurile completate de utilizator
// (custom_name, device_type, location, notes, color).
func (d *DB) UpsertDevice(projectID, scanID int64, o DeviceObservation) (int64, error) {
	now := Now()
	key, err := d.deviceIPKey(projectID, o)
	if err != nil {
		return 0, err
	}
	_, err = d.Exec(`
		INSERT INTO devices(project_id, mac, ip_key, last_ip, hostname, vendor, detected_type, os_guess, first_seen, last_seen)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project_id, mac, ip_key) DO UPDATE SET
			last_ip = excluded.last_ip,
			hostname = CASE WHEN excluded.hostname <> '' THEN excluded.hostname ELSE devices.hostname END,
			vendor = CASE WHEN excluded.vendor <> '' THEN excluded.vendor ELSE devices.vendor END,
			detected_type = excluded.detected_type,
			os_guess = CASE WHEN excluded.os_guess <> '' THEN excluded.os_guess ELSE devices.os_guess END,
			last_seen = excluded.last_seen`,
		projectID, o.MAC, key, o.IP, o.Hostname, o.Vendor, o.DetectedType, o.OSGuess, now, now)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := d.QueryRow(`SELECT id FROM devices WHERE project_id = ? AND mac = ? AND ip_key = ?`, projectID, o.MAC, key).Scan(&id); err != nil {
		return 0, err
	}
	_, err = d.Exec(`INSERT INTO device_observations(scan_id, device_id, ip, open_ports, services, latency_ms) VALUES (?,?,?,?,?,?)`,
		scanID, id, o.IP, o.OpenPorts, o.Services, o.LatencyMS)
	return id, err
}

const deviceCols = `d.id, d.project_id, d.mac, d.ip_key, COALESCE(d.last_ip,''), COALESCE(d.hostname,''), COALESCE(d.vendor,''),
	COALESCE(d.detected_type,''), COALESCE(d.os_guess,''), COALESCE(d.custom_name,''), COALESCE(d.device_type,''),
	COALESCE(d.location,''), COALESCE(d.notes,''), COALESCE(d.color,''), COALESCE(d.first_seen,''), COALESCE(d.last_seen,'')`

func (d *DB) ListDevices(projectID int64) ([]Device, error) {
	lastScan := d.lastScanID(projectID, "lan")
	var prevScan int64
	d.QueryRow(`SELECT id FROM scans WHERE project_id = ? AND type IN ('lan','import') AND status = 'done' AND id < ? ORDER BY id DESC LIMIT 1`, projectID, lastScan).Scan(&prevScan)
	rows, err := d.Query(`SELECT `+deviceCols+`,
		COALESCE(o.open_ports,'[]'), COALESCE(o.services,'{}'), COALESCE(o.latency_ms,0),
		EXISTS(SELECT 1 FROM device_observations x WHERE x.device_id = d.id AND x.scan_id = ?),
		NOT EXISTS(SELECT 1 FROM device_observations x WHERE x.device_id = d.id AND x.scan_id < ?)
		FROM devices d
		LEFT JOIN device_observations o ON o.id = (SELECT MAX(id) FROM device_observations WHERE device_id = d.id)
		WHERE d.project_id = ? ORDER BY d.last_ip`, lastScan, lastScan, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var v Device
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.MAC, &v.IPKey, &v.LastIP, &v.Hostname, &v.Vendor, &v.DetectedType, &v.OSGuess,
			&v.CustomName, &v.DeviceType, &v.Location, &v.Notes, &v.Color, &v.FirstSeen, &v.LastSeen,
			&v.OpenPorts, &v.Services, &v.LatencyMS, &v.Online, &v.New); err != nil {
			return nil, err
		}
		// „Nou” are sens doar dacă a existat un scan LAN anterior.
		v.New = v.New && v.Online && prevScan > 0
		out = append(out, v)
	}
	sortByIP(out)
	return out, rows.Err()
}

func (d *DB) GetDevice(id int64) (*Device, error) {
	var v Device
	err := d.QueryRow(`SELECT `+deviceCols+` FROM devices d WHERE d.id = ?`, id).Scan(&v.ID, &v.ProjectID, &v.MAC, &v.IPKey, &v.LastIP, &v.Hostname,
		&v.Vendor, &v.DetectedType, &v.OSGuess, &v.CustomName, &v.DeviceType, &v.Location, &v.Notes, &v.Color, &v.FirstSeen, &v.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, err
}

// PatchDevice actualizează doar câmpurile editabile de utilizator prezente în map.
func (d *DB) PatchDevice(id int64, fields map[string]any) error {
	return d.patch("devices", id, fields, []string{"custom_name", "device_type", "location", "notes", "color"})
}

// CreateManualDevice adaugă un dispozitiv introdus manual (fără scan).
func (d *DB) CreateManualDevice(projectID int64, mac, ip, name, typ string) (int64, error) {
	now := Now()
	res, err := d.Exec(`INSERT INTO devices(project_id, mac, last_ip, custom_name, device_type, first_seen, last_seen) VALUES (?,?,?,?,?,?,?)`,
		projectID, mac, ip, name, typ, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) DeleteDevice(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM device_observations WHERE device_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE bluetooth_devices SET device_id = NULL, link_status = 'none' WHERE device_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE wifi_networks SET ap_device_id = NULL, ap_link_status = 'none' WHERE ap_device_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM devices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := mustAffect(res); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- Wi-Fi ----------

// WifiObservation e o rețea văzută la un scan.
type WifiObservation struct {
	SSID      string
	BSSID     string
	Band      string
	Channel   int
	Frequency int
	Security  string
	Vendor    string
	SignalDBM int
}

func (d *DB) UpsertWifi(projectID, scanID int64, surveyPointID *int64, o WifiObservation) (int64, error) {
	now := Now()
	_, err := d.Exec(`
		INSERT INTO wifi_networks(project_id, ssid, bssid, band, channel, frequency, security, vendor, first_seen, last_seen)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project_id, bssid) DO UPDATE SET
			ssid = excluded.ssid, band = excluded.band, channel = excluded.channel,
			frequency = excluded.frequency, security = excluded.security,
			vendor = CASE WHEN excluded.vendor <> '' THEN excluded.vendor ELSE wifi_networks.vendor END,
			last_seen = excluded.last_seen`,
		projectID, o.SSID, o.BSSID, o.Band, o.Channel, o.Frequency, o.Security, o.Vendor, now, now)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := d.QueryRow(`SELECT id FROM wifi_networks WHERE project_id = ? AND bssid = ?`, projectID, o.BSSID).Scan(&id); err != nil {
		return 0, err
	}
	_, err = d.Exec(`INSERT INTO wifi_observations(scan_id, wifi_id, signal_dbm, survey_point_id) VALUES (?,?,?,?)`, scanID, id, o.SignalDBM, surveyPointID)
	return id, err
}

const wifiCols = `w.id, w.project_id, COALESCE(w.ssid,''), w.bssid, COALESCE(w.band,''), COALESCE(w.channel,0), COALESCE(w.frequency,0),
	COALESCE(w.security,''), COALESCE(w.vendor,''), w.ap_device_id, COALESCE(w.ap_link_status,'none'), COALESCE(w.label,''),
	COALESCE(w.location,''), COALESCE(w.notes,''), COALESCE(w.first_seen,''), COALESCE(w.last_seen,'')`

func (d *DB) ListWifi(projectID int64) ([]WifiNetwork, error) {
	lastScan := d.lastWifiScanID(projectID)
	rows, err := d.Query(`SELECT `+wifiCols+`,
		(SELECT signal_dbm FROM wifi_observations o WHERE o.wifi_id = w.id ORDER BY o.id DESC LIMIT 1),
		EXISTS(SELECT 1 FROM wifi_observations o WHERE o.wifi_id = w.id AND o.scan_id = ?)
		FROM wifi_networks w WHERE w.project_id = ? ORDER BY w.ssid, w.band, w.bssid`, lastScan, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WifiNetwork{}
	for rows.Next() {
		var w WifiNetwork
		var ap sql.NullInt64
		var sig sql.NullInt64
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.SSID, &w.BSSID, &w.Band, &w.Channel, &w.Frequency, &w.Security, &w.Vendor,
			&ap, &w.APLinkStatus, &w.Label, &w.Location, &w.Notes, &w.FirstSeen, &w.LastSeen, &sig, &w.Visible); err != nil {
			return nil, err
		}
		if ap.Valid {
			w.APDeviceID = &ap.Int64
		}
		if sig.Valid {
			s := int(sig.Int64)
			w.SignalDBM = &s
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (d *DB) lastWifiScanID(projectID int64) int64 {
	var id int64
	d.QueryRow(`SELECT id FROM scans WHERE project_id = ? AND type IN ('wifi','survey') AND status = 'done' ORDER BY id DESC LIMIT 1`, projectID).Scan(&id)
	return id
}

func (d *DB) GetWifi(id int64) (*WifiNetwork, error) {
	var w WifiNetwork
	var ap sql.NullInt64
	err := d.QueryRow(`SELECT `+wifiCols+` FROM wifi_networks w WHERE w.id = ?`, id).Scan(&w.ID, &w.ProjectID, &w.SSID, &w.BSSID, &w.Band,
		&w.Channel, &w.Frequency, &w.Security, &w.Vendor, &ap, &w.APLinkStatus, &w.Label, &w.Location, &w.Notes, &w.FirstSeen, &w.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if ap.Valid {
		w.APDeviceID = &ap.Int64
	}
	return &w, err
}

func (d *DB) PatchWifi(id int64, fields map[string]any) error {
	return d.patch("wifi_networks", id, fields, []string{"label", "location", "notes", "ap_device_id", "ap_link_status"})
}

// SetInferredAP setează o asociere inferată doar dacă utilizatorul nu a decis deja.
func (d *DB) SetInferredAP(wifiID, deviceID int64) error {
	_, err := d.Exec(`UPDATE wifi_networks SET ap_device_id = ?, ap_link_status = 'inferat'
		WHERE id = ? AND COALESCE(ap_link_status,'none') IN ('none','inferat')`, deviceID, wifiID)
	return err
}

// ---------- Survey ----------

func (d *DB) CreateSurveyPoint(projectID int64, mapID *int64, label string, x, y float64) (int64, error) {
	res, err := d.Exec(`INSERT INTO survey_points(project_id, map_id, label, x, y, created_at) VALUES (?,?,?,?,?,?)`, projectID, mapID, label, x, y, Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) ListSurveyPoints(projectID int64) ([]SurveyPoint, error) {
	rows, err := d.Query(`SELECT id, project_id, map_id, COALESCE(label,''), x, y, created_at FROM survey_points WHERE project_id = ? ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	out := []SurveyPoint{}
	for rows.Next() {
		var p SurveyPoint
		var m sql.NullInt64
		if err := rows.Scan(&p.ID, &p.ProjectID, &m, &p.Label, &p.X, &p.Y, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if m.Valid {
			p.MapID = &m.Int64
		}
		out = append(out, p)
	}
	rows.Close()
	for i := range out {
		r, err := d.surveyReadings(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Readings = r
	}
	return out, nil
}

func (d *DB) surveyReadings(pointID int64) ([]SurveyReading, error) {
	rows, err := d.Query(`SELECT w.id, COALESCE(w.ssid,''), w.bssid, COALESCE(w.band,''), COALESCE(w.channel,0), COALESCE(o.signal_dbm,-100)
		FROM wifi_observations o JOIN wifi_networks w ON w.id = o.wifi_id
		WHERE o.survey_point_id = ? ORDER BY o.signal_dbm DESC`, pointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SurveyReading{}
	for rows.Next() {
		var r SurveyReading
		if err := rows.Scan(&r.WifiID, &r.SSID, &r.BSSID, &r.Band, &r.Channel, &r.SignalDBM); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) PatchSurveyPoint(id int64, fields map[string]any) error {
	return d.patch("survey_points", id, fields, []string{"label", "x", "y"})
}

func (d *DB) DeleteSurveyPoint(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE wifi_observations SET survey_point_id = NULL WHERE survey_point_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM survey_points WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := mustAffect(res); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- Hărți ----------

// DefaultScene e scena unei hărți noi.
const DefaultScene = `{"schema":1,"grid":{"size":20,"snap":true,"visible":true},"scale":{"unitsPerMeter":40},"view":{"zoom":1,"panX":0,"panY":0},` +
	`"layers":[{"id":"background","visible":true,"locked":false},{"id":"walls","visible":true,"locked":false},{"id":"links","visible":true,"locked":false},` +
	`{"id":"nodes","visible":true,"locked":false},{"id":"survey","visible":true,"locked":false},{"id":"labels","visible":true,"locked":false}],` +
	`"rooms":[],"walls":[],"nodes":[],"links":[],"texts":[],"doors":[],"surveyPoints":[]}`

func (d *DB) ListMaps(projectID int64) ([]Map, error) {
	rows, err := d.Query(`SELECT id, project_id, name, version, updated_at FROM maps WHERE project_id = ? ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Map{}
	for rows.Next() {
		var m Map
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Name, &m.Version, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (d *DB) GetMap(id int64) (*Map, error) {
	var m Map
	var bg sql.NullString
	err := d.QueryRow(`SELECT id, project_id, name, scene_json, background, version, updated_at FROM maps WHERE id = ?`, id).
		Scan(&m.ID, &m.ProjectID, &m.Name, &m.SceneJSON, &bg, &m.Version, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	m.Background = ns(bg)
	return &m, err
}

func (d *DB) CreateMap(projectID int64, name, scene string) (*Map, error) {
	if scene == "" {
		scene = DefaultScene
	}
	res, err := d.Exec(`INSERT INTO maps(project_id, name, scene_json, version, updated_at) VALUES (?,?,?,1,?)`, projectID, name, scene, Now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetMap(id)
}

// SaveMap salvează scena cu optimistic locking: reușește doar dacă versiunea
// trimisă e cea din baza de date, apoi o incrementează.
func (d *DB) SaveMap(id int64, name *string, scene string, background *string, version int) (int, error) {
	q := `UPDATE maps SET scene_json = ?, version = version + 1, updated_at = ?`
	args := []any{scene, Now()}
	if name != nil {
		q += `, name = ?`
		args = append(args, *name)
	}
	if background != nil {
		q += `, background = ?`
		args = append(args, *background)
	}
	q += ` WHERE id = ? AND version = ?`
	args = append(args, id, version)
	res, err := d.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := d.GetMap(id); err != nil {
			return 0, err
		}
		return 0, ErrVersionConflict
	}
	return version + 1, nil
}

func (d *DB) RenameMap(id int64, name string) error {
	res, err := d.Exec(`UPDATE maps SET name = ?, updated_at = ? WHERE id = ?`, name, Now(), id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

func (d *DB) DeleteMap(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE survey_points SET map_id = NULL WHERE map_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM maps WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := mustAffect(res); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- utilitare ----------

func (d *DB) patch(table string, id int64, fields map[string]any, allowed []string) error {
	sets := []string{}
	args := []any{}
	for _, col := range allowed {
		v, ok := fields[col]
		if !ok {
			continue
		}
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	res, err := d.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

func mustAffect(res sql.Result) error {
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- Setări ----------

func (d *DB) GetSetting(key string) string {
	var v string
	d.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.Exec(`INSERT INTO settings(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// ---------- Bluetooth ----------

type BluetoothDevice struct {
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"project_id"`
	MAC          string `json:"mac"`
	Name         string `json:"name"`
	AddressType  string `json:"address_type"`
	Kind         string `json:"kind"`
	Manufacturer string `json:"manufacturer"`
	Paired       bool   `json:"paired"`
	DetectedType string `json:"detected_type"`
	DeviceType   string `json:"device_type"`
	CustomName   string `json:"custom_name"`
	Location     string `json:"location"`
	Notes        string `json:"notes"`
	DeviceID     *int64 `json:"device_id"`
	LinkStatus   string `json:"link_status"`
	FirstSeen    string `json:"first_seen"`
	LastSeen     string `json:"last_seen"`
	RSSI         *int   `json:"rssi"`    // din ultima observație
	Visible      bool   `json:"visible"` // văzut la ultimul scan Bluetooth
}

// BluetoothObservation e un dispozitiv văzut la un scan.
type BluetoothObservation struct {
	MAC, Name, AddressType, Kind, Manufacturer, DetectedType string
	Paired                                                   bool
	RSSI                                                     *int
}

// UpsertBluetooth: la fel ca la LAN, câmpurile utilizatorului nu se ating.
func (d *DB) UpsertBluetooth(projectID, scanID int64, o BluetoothObservation) (int64, error) {
	now := Now()
	_, err := d.Exec(`
		INSERT INTO bluetooth_devices(project_id, mac, name, address_type, kind, manufacturer, paired, detected_type, first_seen, last_seen)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project_id, mac) DO UPDATE SET
			name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE bluetooth_devices.name END,
			address_type = excluded.address_type,
			kind = CASE WHEN excluded.kind <> '' THEN excluded.kind ELSE bluetooth_devices.kind END,
			manufacturer = CASE WHEN excluded.manufacturer <> '' THEN excluded.manufacturer ELSE bluetooth_devices.manufacturer END,
			paired = excluded.paired,
			detected_type = excluded.detected_type,
			last_seen = excluded.last_seen`,
		projectID, o.MAC, o.Name, o.AddressType, o.Kind, o.Manufacturer, o.Paired, o.DetectedType, now, now)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := d.QueryRow(`SELECT id FROM bluetooth_devices WHERE project_id = ? AND mac = ?`, projectID, o.MAC).Scan(&id); err != nil {
		return 0, err
	}
	_, err = d.Exec(`INSERT INTO bluetooth_observations(scan_id, bt_id, rssi) VALUES (?,?,?)`, scanID, id, o.RSSI)
	return id, err
}

const btCols = `b.id, b.project_id, b.mac, COALESCE(b.name,''), COALESCE(b.address_type,''), COALESCE(b.kind,''), COALESCE(b.manufacturer,''),
	b.paired, COALESCE(b.detected_type,''), COALESCE(b.device_type,''), COALESCE(b.custom_name,''), COALESCE(b.location,''),
	COALESCE(b.notes,''), b.device_id, COALESCE(b.link_status,'none'), COALESCE(b.first_seen,''), COALESCE(b.last_seen,'')`

func (d *DB) ListBluetooth(projectID int64) ([]BluetoothDevice, error) {
	lastScan := d.lastScanID(projectID, "bluetooth")
	rows, err := d.Query(`SELECT `+btCols+`,
		(SELECT rssi FROM bluetooth_observations o WHERE o.bt_id = b.id ORDER BY o.id DESC LIMIT 1),
		EXISTS(SELECT 1 FROM bluetooth_observations o WHERE o.bt_id = b.id AND o.scan_id = ?)
		FROM bluetooth_devices b WHERE b.project_id = ? ORDER BY (b.name IS NULL OR b.name = ''), b.name, b.mac`, lastScan, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BluetoothDevice{}
	for rows.Next() {
		var b BluetoothDevice
		var dev, rssi sql.NullInt64
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.MAC, &b.Name, &b.AddressType, &b.Kind, &b.Manufacturer, &b.Paired,
			&b.DetectedType, &b.DeviceType, &b.CustomName, &b.Location, &b.Notes, &dev, &b.LinkStatus, &b.FirstSeen, &b.LastSeen,
			&rssi, &b.Visible); err != nil {
			return nil, err
		}
		if dev.Valid {
			b.DeviceID = &dev.Int64
		}
		if rssi.Valid {
			v := int(rssi.Int64)
			b.RSSI = &v
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (d *DB) GetBluetooth(id int64) (*BluetoothDevice, error) {
	var b BluetoothDevice
	var dev sql.NullInt64
	err := d.QueryRow(`SELECT `+btCols+` FROM bluetooth_devices b WHERE b.id = ?`, id).Scan(&b.ID, &b.ProjectID, &b.MAC, &b.Name,
		&b.AddressType, &b.Kind, &b.Manufacturer, &b.Paired, &b.DetectedType, &b.DeviceType, &b.CustomName, &b.Location, &b.Notes,
		&dev, &b.LinkStatus, &b.FirstSeen, &b.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if dev.Valid {
		b.DeviceID = &dev.Int64
	}
	return &b, err
}

func (d *DB) PatchBluetooth(id int64, fields map[string]any) error {
	return d.patch("bluetooth_devices", id, fields, []string{"custom_name", "device_type", "location", "notes", "device_id", "link_status"})
}

// SetInferredBTLink leagă un dispozitiv Bluetooth de unul din LAN (același nume),
// doar dacă utilizatorul nu a decis deja.
func (d *DB) SetInferredBTLink(btID, deviceID int64) error {
	_, err := d.Exec(`UPDATE bluetooth_devices SET device_id = ?, link_status = 'inferat'
		WHERE id = ? AND COALESCE(link_status,'none') IN ('none','inferat')`, deviceID, btID)
	return err
}

func (d *DB) DeleteBluetooth(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM bluetooth_observations WHERE bt_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM bluetooth_devices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := mustAffect(res); err != nil {
		return err
	}
	return tx.Commit()
}
