// Package export produce CSV, backup JSON (și restaurare), SVG și raportul HTML.
package export

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"webrt-network-mapper/internal/db"
)

// Format identifică fișierele de backup / instantaneu.
const Format = "webrt-network-mapper"

// Backup e exportul complet al unui proiect. Același format e folosit
// pentru instantaneele salvate în folder (.netmap.json).
type Backup struct {
	Format       string               `json:"format"`
	Version      int                  `json:"version"`
	Name         string               `json:"name,omitempty"`
	Note         string               `json:"note,omitempty"`
	TakenAt      string               `json:"taken_at"`
	Project      db.Project           `json:"project"`
	Scans        []db.Scan            `json:"scans"`
	Devices      []db.Device          `json:"devices"`
	Wifi         []db.WifiNetwork     `json:"wifi"`
	Bluetooth    []db.BluetoothDevice `json:"bluetooth"`
	SurveyPoints []db.SurveyPoint     `json:"survey_points"`
	Maps         []db.Map             `json:"maps"`
}

// BuildBackup colectează toate datele proiectului.
func BuildBackup(d *db.DB, projectID int64) (*Backup, error) {
	p, err := d.GetProject(projectID)
	if err != nil {
		return nil, err
	}
	b := &Backup{Format: Format, Version: 1, TakenAt: time.Now().Format(time.RFC3339), Project: *p}
	if b.Scans, err = d.ListScans(projectID); err != nil {
		return nil, err
	}
	if b.Devices, err = d.ListDevices(projectID); err != nil {
		return nil, err
	}
	if b.Wifi, err = d.ListWifi(projectID); err != nil {
		return nil, err
	}
	if b.Bluetooth, err = d.ListBluetooth(projectID); err != nil {
		return nil, err
	}
	if b.SurveyPoints, err = d.ListSurveyPoints(projectID); err != nil {
		return nil, err
	}
	maps, err := d.ListMaps(projectID)
	if err != nil {
		return nil, err
	}
	for _, m := range maps {
		full, err := d.GetMap(m.ID)
		if err != nil {
			return nil, err
		}
		b.Maps = append(b.Maps, *full)
	}
	return b, nil
}

// ParseBackup validează un fișier de backup.
func ParseBackup(data []byte) (*Backup, error) {
	var b Backup
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("fișier JSON invalid: %w", err)
	}
	if b.Format != Format {
		return nil, errors.New("fișierul nu este un backup webrt-network-mapper")
	}
	return &b, nil
}

// Restore creează un proiect nou din backup și întoarce ID-ul lui.
// ID-urile interne sunt remapate, inclusiv referințele din scene.
func Restore(d *db.DB, b *Backup, newName string) (int64, error) {
	if newName == "" {
		newName = b.Project.Name
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := db.Now()
	res, err := tx.Exec(`INSERT INTO projects(name, address, notes, created_at, updated_at) VALUES (?,?,?,?,?)`,
		newName, b.Project.Address, b.Project.Notes, now, now)
	if err != nil {
		return 0, err
	}
	pid, _ := res.LastInsertId()

	// Un scan sintetic ține observațiile importate (porturi, semnal).
	res, err = tx.Exec(`INSERT INTO scans(project_id, type, status, progress, message, started_at, finished_at, subnet, gateway, dns, interface, method)
		VALUES (?, 'import', 'done', 100, ?, ?, ?, ?, ?, ?, ?, 'import')`,
		pid, "Importat din backup "+b.TakenAt, now, now, lastLAN(b).Subnet, lastLAN(b).Gateway, lastLAN(b).DNS, lastLAN(b).Interface)
	if err != nil {
		return 0, err
	}
	scanID, _ := res.LastInsertId()

	devMap := map[int64]int64{}
	for _, v := range b.Devices {
		r, err := tx.Exec(`INSERT INTO devices(project_id, mac, ip_key, last_ip, hostname, vendor, detected_type, os_guess, custom_name, device_type, location, notes, color, first_seen, last_seen)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, pid, v.MAC, v.IPKey, v.LastIP, v.Hostname, v.Vendor, v.DetectedType, v.OSGuess,
			v.CustomName, v.DeviceType, v.Location, v.Notes, v.Color, v.FirstSeen, v.LastSeen)
		if err != nil {
			return 0, err
		}
		nid, _ := r.LastInsertId()
		devMap[v.ID] = nid
		if _, err := tx.Exec(`INSERT INTO device_observations(scan_id, device_id, ip, open_ports, services, latency_ms) VALUES (?,?,?,?,?,?)`,
			scanID, nid, v.LastIP, v.OpenPorts, v.Services, v.LatencyMS); err != nil {
			return 0, err
		}
	}

	mapMap := map[int64]int64{}
	for _, m := range b.Maps {
		r, err := tx.Exec(`INSERT INTO maps(project_id, name, scene_json, background, version, updated_at) VALUES (?,?,?,?,1,?)`,
			pid, m.Name, m.SceneJSON, nullIfEmpty(m.Background), now)
		if err != nil {
			return 0, err
		}
		nid, _ := r.LastInsertId()
		mapMap[m.ID] = nid
	}

	spMap := map[int64]int64{}
	for _, sp := range b.SurveyPoints {
		var mid any
		if sp.MapID != nil {
			if v, ok := mapMap[*sp.MapID]; ok {
				mid = v
			}
		}
		r, err := tx.Exec(`INSERT INTO survey_points(project_id, map_id, label, x, y, created_at) VALUES (?,?,?,?,?,?)`, pid, mid, sp.Label, sp.X, sp.Y, sp.CreatedAt)
		if err != nil {
			return 0, err
		}
		nid, _ := r.LastInsertId()
		spMap[sp.ID] = nid
	}

	wifiMap := map[int64]int64{}
	for _, w := range b.Wifi {
		var ap any
		if w.APDeviceID != nil {
			if v, ok := devMap[*w.APDeviceID]; ok {
				ap = v
			}
		}
		r, err := tx.Exec(`INSERT INTO wifi_networks(project_id, ssid, bssid, band, channel, frequency, security, vendor, ap_device_id, ap_link_status, label, location, notes, first_seen, last_seen)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, pid, w.SSID, w.BSSID, w.Band, w.Channel, w.Frequency, w.Security, w.Vendor,
			ap, w.APLinkStatus, w.Label, w.Location, w.Notes, w.FirstSeen, w.LastSeen)
		if err != nil {
			return 0, err
		}
		nid, _ := r.LastInsertId()
		wifiMap[w.ID] = nid
		if w.SignalDBM != nil {
			if _, err := tx.Exec(`INSERT INTO wifi_observations(scan_id, wifi_id, signal_dbm) VALUES (?,?,?)`, scanID, nid, *w.SignalDBM); err != nil {
				return 0, err
			}
		}
	}
	for _, sp := range b.SurveyPoints {
		for _, rd := range sp.Readings {
			wid, ok := wifiMap[rd.WifiID]
			if !ok {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO wifi_observations(scan_id, wifi_id, signal_dbm, survey_point_id) VALUES (?,?,?,?)`, scanID, wid, rd.SignalDBM, spMap[sp.ID]); err != nil {
				return 0, err
			}
		}
	}

	btMap := map[int64]int64{}
	for _, bt := range b.Bluetooth {
		var dev any
		if bt.DeviceID != nil {
			if v, ok := devMap[*bt.DeviceID]; ok {
				dev = v
			}
		}
		r, err := tx.Exec(`INSERT INTO bluetooth_devices(project_id, mac, name, address_type, kind, manufacturer, paired, detected_type, device_type, custom_name, location, notes, device_id, link_status, first_seen, last_seen)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, pid, bt.MAC, bt.Name, bt.AddressType, bt.Kind, bt.Manufacturer, bt.Paired, bt.DetectedType,
			bt.DeviceType, bt.CustomName, bt.Location, bt.Notes, dev, bt.LinkStatus, bt.FirstSeen, bt.LastSeen)
		if err != nil {
			return 0, err
		}
		nid, _ := r.LastInsertId()
		btMap[bt.ID] = nid
		if bt.RSSI != nil {
			if _, err := tx.Exec(`INSERT INTO bluetooth_observations(scan_id, bt_id, rssi) VALUES (?,?,?)`, scanID, nid, *bt.RSSI); err != nil {
				return 0, err
			}
		}
	}

	// Scenele conțin device_id, bt_id și survey_point_id vechi: le remapăm.
	for oldID, newID := range mapMap {
		var scene string
		if err := tx.QueryRow(`SELECT scene_json FROM maps WHERE id = ?`, newID).Scan(&scene); err != nil {
			return 0, err
		}
		fixed, err := remapScene(scene, devMap, spMap, btMap)
		if err != nil {
			return 0, fmt.Errorf("harta %d: %w", oldID, err)
		}
		if _, err := tx.Exec(`UPDATE maps SET scene_json = ? WHERE id = ?`, fixed, newID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return pid, nil
}

func remapScene(scene string, devMap, spMap, btMap map[int64]int64) (string, error) {
	var s map[string]any
	if err := json.Unmarshal([]byte(scene), &s); err != nil {
		return "", err
	}
	remap := func(list any, key string, m map[int64]int64) {
		arr, _ := list.([]any)
		for _, it := range arr {
			o, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if f, ok := o[key].(float64); ok {
				if v, ok := m[int64(f)]; ok {
					o[key] = v
				} else {
					delete(o, key)
				}
			}
		}
	}
	remap(s["nodes"], "device_id", devMap)
	remap(s["nodes"], "bt_id", btMap)
	remap(s["surveyPoints"], "survey_point_id", spMap)
	out, err := json.Marshal(s)
	return string(out), err
}

func lastLAN(b *Backup) db.Scan {
	for _, s := range b.Scans {
		if (s.Type == "lan" || s.Type == "import") && s.Subnet != "" {
			return s
		}
	}
	return db.Scan{}
}

func nullIfEmpty(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
