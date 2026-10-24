package db

import (
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) (*DB, int64) {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	res, err := d.Exec(`INSERT INTO projects(name, created_at, updated_at) VALUES ('p', ?, ?)`, Now(), Now())
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()
	return d, pid
}

func newScan(t *testing.T, d *DB, pid int64) int64 {
	t.Helper()
	res, err := d.Exec(`INSERT INTO scans(project_id, type, status, started_at) VALUES (?, 'lan', 'done', ?)`, pid, Now())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestUpsertDeviceSharedMAC(t *testing.T) {
	d, pid := openTest(t)
	const mac = "F4:B1:9C:16:31:B5"

	// Scan 1: MAC-ul răspunde pentru un singur IP; utilizatorul îl denumește.
	s1 := newScan(t, d, pid)
	id15, err := d.UpsertDevice(pid, s1, DeviceObservation{MAC: mac, IP: "192.168.1.15"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.PatchDevice(id15, map[string]any{"custom_name": "Cameră 1"}); err != nil {
		t.Fatal(err)
	}

	// Scan 2: același MAC pe .10 și .15 → două dispozitive; .15 își păstrează numele.
	s2 := newScan(t, d, pid)
	a, err := d.UpsertDevice(pid, s2, DeviceObservation{MAC: mac, IP: "192.168.1.10", SharedMAC: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.UpsertDevice(pid, s2, DeviceObservation{MAC: mac, IP: "192.168.1.15", SharedMAC: true})
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf(".10 și .15 au fost comasate în dispozitivul %d", a)
	}
	if b != id15 {
		t.Fatalf(".15 trebuia să rămână dispozitivul %d, e %d", id15, b)
	}
	devs, err := d.ListDevices(pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("aștept 2 dispozitive, am %d", len(devs))
	}
	if devs[1].LastIP != "192.168.1.15" || devs[1].CustomName != "Cameră 1" || devs[1].IPKey != "192.168.1.15" {
		t.Fatalf("dispozitivul .15 greșit: %+v", devs[1])
	}

	// Scan 3: la fel → aceleași dispozitive, fără duplicate.
	s3 := newScan(t, d, pid)
	a3, _ := d.UpsertDevice(pid, s3, DeviceObservation{MAC: mac, IP: "192.168.1.10", SharedMAC: true})
	b3, _ := d.UpsertDevice(pid, s3, DeviceObservation{MAC: mac, IP: "192.168.1.15", SharedMAC: true})
	if a3 != a || b3 != b {
		t.Fatalf("identitate instabilă: %d/%d față de %d/%d", a3, b3, a, b)
	}

	// Scan 4: MAC-ul nu mai e partajat → rămâne pe dispozitivul IP-ului respectiv.
	s4 := newScan(t, d, pid)
	b4, _ := d.UpsertDevice(pid, s4, DeviceObservation{MAC: mac, IP: "192.168.1.15"})
	if b4 != b {
		t.Fatalf("după ce MAC-ul nu mai e partajat, .15 trebuia să rămână %d, e %d", b, b4)
	}
	if devs, _ := d.ListDevices(pid); len(devs) != 2 {
		t.Fatalf("aștept tot 2 dispozitive, am %d", len(devs))
	}
}

func TestUpsertDeviceNormalMACFollowsIPChange(t *testing.T) {
	d, pid := openTest(t)
	const mac = "9C:53:22:6A:39:4F"
	id1, _ := d.UpsertDevice(pid, newScan(t, d, pid), DeviceObservation{MAC: mac, IP: "192.168.1.19"})
	id2, _ := d.UpsertDevice(pid, newScan(t, d, pid), DeviceObservation{MAC: mac, IP: "192.168.1.40"})
	if id1 != id2 {
		t.Fatalf("schimbarea IP-ului (DHCP) a creat un dispozitiv nou: %d → %d", id1, id2)
	}
}
