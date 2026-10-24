package snapshot

import (
	"encoding/json"
	"fmt"
	"sort"

	"webrt-network-mapper/internal/db"
	"webrt-network-mapper/internal/export"
)

// Change e o modificare de câmp.
type Change struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// DeviceDiff descrie un dispozitiv în comparație (cheia e MAC-ul).
type DeviceDiff struct {
	MAC     string   `json:"mac"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	IP      string   `json:"ip"`
	Vendor  string   `json:"vendor"`
	Changes []Change `json:"changes,omitempty"`
}

// WifiDiff descrie un BSSID în comparație.
type WifiDiff struct {
	BSSID   string   `json:"bssid"`
	SSID    string   `json:"ssid"`
	Band    string   `json:"band"`
	Channel int      `json:"channel"`
	Changes []Change `json:"changes,omitempty"`
}

// MapDiff descrie diferențele de pe o hartă (potrivire după nume).
type MapDiff struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"` // added | removed | changed | same
	NodesBefore  int      `json:"nodes_before"`
	NodesAfter   int      `json:"nodes_after"`
	LinksBefore  int      `json:"links_before"`
	LinksAfter   int      `json:"links_after"`
	AddedNodes   []string `json:"added_nodes"`
	RemovedNodes []string `json:"removed_nodes"`
	MovedNodes   []string `json:"moved_nodes"`
}

// Side e rezumatul unei părți a comparației.
type Side struct {
	Label   string `json:"label"`
	TakenAt string `json:"taken_at"`
	Devices int    `json:"devices"`
	Online  int    `json:"online"`
	Wifi    int    `json:"wifi"`
	SSIDs   int    `json:"ssids"`
	BT      int    `json:"bt"`
}

// Diff e rezultatul comparării a două instantanee (A = înainte, B = după).
type Diff struct {
	A              Side         `json:"a"`
	B              Side         `json:"b"`
	DevicesAdded   []DeviceDiff `json:"devices_added"`
	DevicesRemoved []DeviceDiff `json:"devices_removed"`
	DevicesChanged []DeviceDiff `json:"devices_changed"`
	DevicesSame    int          `json:"devices_same"`
	WifiAdded      []WifiDiff   `json:"wifi_added"`
	WifiRemoved    []WifiDiff   `json:"wifi_removed"`
	WifiChanged    []WifiDiff   `json:"wifi_changed"`
	WifiSame       int          `json:"wifi_same"`
	BTAdded        []BTDiff     `json:"bt_added"`
	BTRemoved      []BTDiff     `json:"bt_removed"`
	BTChanged      []BTDiff     `json:"bt_changed"`
	Maps           []MapDiff    `json:"maps"`
}

// BTDiff descrie un dispozitiv Bluetooth (doar cele cu nume; anonimele sunt zgomot).
type BTDiff struct {
	MAC     string   `json:"mac"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Changes []Change `json:"changes,omitempty"`
}

// Compare compară două backup-uri.
// deviceKey identifică un dispozitiv între instantanee: MAC-ul, plus IP-ul când MAC-ul e partajat.
func deviceKey(v db.Device) string {
	if v.IPKey != "" {
		return v.MAC + "@" + v.IPKey
	}
	return v.MAC
}

func Compare(a, b *export.Backup, labelA, labelB string) *Diff {
	d := &Diff{A: side(a, labelA), B: side(b, labelB)}
	d.DevicesAdded, d.DevicesRemoved, d.DevicesChanged = []DeviceDiff{}, []DeviceDiff{}, []DeviceDiff{}
	d.WifiAdded, d.WifiRemoved, d.WifiChanged = []WifiDiff{}, []WifiDiff{}, []WifiDiff{}
	d.Maps = []MapDiff{}

	// --- dispozitive, după MAC ---
	am := map[string]db.Device{}
	for _, v := range a.Devices {
		am[deviceKey(v)] = v
	}
	bm := map[string]db.Device{}
	for _, v := range b.Devices {
		bm[deviceKey(v)] = v
		old, ok := am[deviceKey(v)]
		if !ok {
			d.DevicesAdded = append(d.DevicesAdded, devDiff(v, nil))
			continue
		}
		ch := deviceChanges(old, v)
		if len(ch) > 0 {
			d.DevicesChanged = append(d.DevicesChanged, devDiff(v, ch))
		} else {
			d.DevicesSame++
		}
	}
	for _, v := range a.Devices {
		if _, ok := bm[deviceKey(v)]; !ok {
			d.DevicesRemoved = append(d.DevicesRemoved, devDiff(v, nil))
		}
	}

	// --- Wi-Fi, după BSSID ---
	aw := map[string]db.WifiNetwork{}
	for _, w := range a.Wifi {
		aw[w.BSSID] = w
	}
	bw := map[string]bool{}
	for _, w := range b.Wifi {
		bw[w.BSSID] = true
		old, ok := aw[w.BSSID]
		if !ok {
			d.WifiAdded = append(d.WifiAdded, wifiDiff(w, nil))
			continue
		}
		var ch []Change
		add := func(field, label, x, y string) {
			if x != y {
				ch = append(ch, Change{field, label, x, y})
			}
		}
		add("ssid", "SSID", old.SSID, w.SSID)
		add("channel", "Canal", fmt.Sprint(old.Channel), fmt.Sprint(w.Channel))
		add("security", "Securitate", old.Security, w.Security)
		add("label", "Etichetă", old.Label, w.Label)
		add("location", "Locație", old.Location, w.Location)
		if len(ch) > 0 {
			d.WifiChanged = append(d.WifiChanged, wifiDiff(w, ch))
		} else {
			d.WifiSame++
		}
	}
	for _, w := range a.Wifi {
		if !bw[w.BSSID] {
			d.WifiRemoved = append(d.WifiRemoved, wifiDiff(w, nil))
		}
	}

	// --- Bluetooth, după MAC (doar dispozitivele cu nume) ---
	d.BTAdded, d.BTRemoved, d.BTChanged = []BTDiff{}, []BTDiff{}, []BTDiff{}
	named := func(bt db.BluetoothDevice) bool { return bt.Name != "" || bt.CustomName != "" }
	abt := map[string]db.BluetoothDevice{}
	for _, bt := range a.Bluetooth {
		if named(bt) {
			abt[bt.MAC] = bt
		}
	}
	bseen := map[string]bool{}
	for _, bt := range b.Bluetooth {
		if !named(bt) {
			continue
		}
		bseen[bt.MAC] = true
		old, ok := abt[bt.MAC]
		item := BTDiff{MAC: bt.MAC, Name: export.BluetoothName(bt), Type: export.BluetoothType(bt)}
		if !ok {
			d.BTAdded = append(d.BTAdded, item)
			continue
		}
		for _, c := range []Change{
			{"name", "Nume", export.BluetoothName(old), export.BluetoothName(bt)},
			{"type", "Tip", export.BluetoothType(old), export.BluetoothType(bt)},
			{"location", "Locație", old.Location, bt.Location},
		} {
			if c.Before != c.After {
				item.Changes = append(item.Changes, c)
			}
		}
		if len(item.Changes) > 0 {
			d.BTChanged = append(d.BTChanged, item)
		}
	}
	for mac, bt := range abt {
		if !bseen[mac] {
			d.BTRemoved = append(d.BTRemoved, BTDiff{MAC: mac, Name: export.BluetoothName(bt), Type: export.BluetoothType(bt)})
		}
	}

	// --- hărți, după nume ---
	amap := map[string]db.Map{}
	for _, m := range a.Maps {
		amap[m.Name] = m
	}
	seen := map[string]bool{}
	for _, m := range b.Maps {
		seen[m.Name] = true
		old, ok := amap[m.Name]
		if !ok {
			sb := parse(m.SceneJSON)
			d.Maps = append(d.Maps, MapDiff{Name: m.Name, Status: "added", NodesAfter: len(sb.Nodes), LinksAfter: len(sb.Links)})
			continue
		}
		d.Maps = append(d.Maps, mapDiff(m.Name, old.SceneJSON, m.SceneJSON, a, b))
	}
	for _, m := range a.Maps {
		if !seen[m.Name] {
			sa := parse(m.SceneJSON)
			d.Maps = append(d.Maps, MapDiff{Name: m.Name, Status: "removed", NodesBefore: len(sa.Nodes), LinksBefore: len(sa.Links)})
		}
	}

	sortDev := func(s []DeviceDiff) {
		sort.Slice(s, func(i, j int) bool { return s[i].IP < s[j].IP })
	}
	sortDev(d.DevicesAdded)
	sortDev(d.DevicesRemoved)
	sortDev(d.DevicesChanged)
	return d
}

func side(b *export.Backup, label string) Side {
	s := Side{Label: label, TakenAt: b.TakenAt, Devices: len(b.Devices), Wifi: len(b.Wifi)}
	ssids := map[string]bool{}
	for _, w := range b.Wifi {
		ssids[w.SSID] = true
	}
	s.SSIDs = len(ssids)
	for _, bt := range b.Bluetooth {
		if bt.Name != "" || bt.CustomName != "" {
			s.BT++
		}
	}
	for _, v := range b.Devices {
		if v.Online {
			s.Online++
		}
	}
	return s
}

func devDiff(v db.Device, ch []Change) DeviceDiff {
	return DeviceDiff{MAC: v.MAC, Name: export.DisplayName(v), Type: export.EffectiveType(v), IP: v.LastIP, Vendor: v.Vendor, Changes: ch}
}

func wifiDiff(w db.WifiNetwork, ch []Change) WifiDiff {
	return WifiDiff{BSSID: w.BSSID, SSID: w.SSID, Band: w.Band, Channel: w.Channel, Changes: ch}
}

func deviceChanges(a, b db.Device) []Change {
	var ch []Change
	add := func(field, label, x, y string) {
		if x != y {
			ch = append(ch, Change{field, label, x, y})
		}
	}
	add("ip", "IP", a.LastIP, b.LastIP)
	add("name", "Nume", a.CustomName, b.CustomName)
	add("type", "Tip", export.EffectiveType(a), export.EffectiveType(b))
	add("location", "Locație", a.Location, b.Location)
	add("hostname", "Hostname", a.Hostname, b.Hostname)
	add("ports", "Porturi", export.PortsString(a.OpenPorts), export.PortsString(b.OpenPorts))
	add("notes", "Observații", a.Notes, b.Notes)
	onl := func(o bool) string {
		if o {
			return "online"
		}
		return "offline"
	}
	add("online", "Stare", onl(a.Online), onl(b.Online))
	return ch
}

type sceneLite struct {
	Nodes []struct {
		ID       string  `json:"id"`
		Label    string  `json:"label"`
		Icon     string  `json:"icon"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		DeviceID *int64  `json:"device_id"`
	} `json:"nodes"`
	Links []json.RawMessage `json:"links"`
}

func parse(js string) sceneLite {
	var s sceneLite
	json.Unmarshal([]byte(js), &s)
	return s
}

// mapDiff potrivește nodurile după MAC-ul dispozitivului legat, altfel după id-ul nodului.
func mapDiff(name, sa, sb string, a, b *export.Backup) MapDiff {
	x, y := parse(sa), parse(sb)
	macA := map[int64]string{}
	for _, v := range a.Devices {
		macA[v.ID] = deviceKey(v)
	}
	macB := map[int64]string{}
	for _, v := range b.Devices {
		macB[v.ID] = deviceKey(v)
	}
	key := func(id string, dev *int64, macs map[int64]string) string {
		if dev != nil {
			if m, ok := macs[*dev]; ok {
				return "mac:" + m
			}
		}
		return "node:" + id
	}
	type pos struct {
		label string
		x, y  float64
	}
	before := map[string]pos{}
	for _, n := range x.Nodes {
		before[key(n.ID, n.DeviceID, macA)] = pos{orLabel(n.Label, n.Icon), n.X, n.Y}
	}
	md := MapDiff{Name: name, NodesBefore: len(x.Nodes), NodesAfter: len(y.Nodes), LinksBefore: len(x.Links), LinksAfter: len(y.Links),
		AddedNodes: []string{}, RemovedNodes: []string{}, MovedNodes: []string{}}
	after := map[string]bool{}
	for _, n := range y.Nodes {
		k := key(n.ID, n.DeviceID, macB)
		after[k] = true
		p, ok := before[k]
		if !ok {
			md.AddedNodes = append(md.AddedNodes, orLabel(n.Label, n.Icon))
		} else if p.x != n.X || p.y != n.Y {
			md.MovedNodes = append(md.MovedNodes, orLabel(n.Label, n.Icon))
		}
	}
	for k, p := range before {
		if !after[k] {
			md.RemovedNodes = append(md.RemovedNodes, p.label)
		}
	}
	sort.Strings(md.RemovedNodes)
	md.Status = "same"
	if len(md.AddedNodes)+len(md.RemovedNodes)+len(md.MovedNodes) > 0 || md.LinksBefore != md.LinksAfter || sa != sb {
		md.Status = "changed"
	}
	return md
}

func orLabel(l, icon string) string {
	if l != "" {
		return l
	}
	if def, ok := export.Icons()[icon]; ok {
		return def.Label
	}
	return icon
}
