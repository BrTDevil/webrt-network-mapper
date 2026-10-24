package export

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"webrt-network-mapper/internal/db"
	"webrt-network-mapper/internal/wifi"
	"webrt-network-mapper/web"
)

// ReportOptions controlează raportul.
type ReportOptions struct {
	Author    string
	AutoPrint bool // deschide dialogul de tipărire (pentru PDF)
}

type reportMap struct {
	Name string
	SVG  template.HTML
}

type reportDevice struct {
	db.Device
	Name, Type, TypeLabel, Ports string
}

type reportGroup struct {
	Location string
	Devices  []reportDevice
}

type reportBSSID struct {
	db.WifiNetwork
	AP     string
	Signal string
}

type reportSSID struct {
	SSID   string
	BSSIDs []reportBSSID
}

type reportBT struct {
	Name, TypeLabel, MAC, Manufacturer, Signal, Link, Location, Notes string
	Visible                                                           bool
}

type reportSurvey struct {
	Label    string
	Readings []db.SurveyReading
}

type reportData struct {
	Title, Date, Author string
	AutoPrint           bool
	Project             db.Project
	Scan                db.Scan
	NDevices, NOnline   int
	NAPs                int
	SSIDs               []string
	Maps                []reportMap
	SSIDGroups          []reportSSID
	Groups              []reportGroup
	Warnings            []wifi.Warning
	Survey              []reportSurvey
	Bluetooth           []reportBT
	BTHidden            int
	CSS                 template.CSS
}

// WriteHTML scrie raportul self-contained (fără resurse externe).
func WriteHTML(w io.Writer, b *Backup, opt ReportOptions) error {
	data := buildReport(b, opt)
	return reportTpl.Execute(w, data)
}

func buildReport(b *Backup, opt ReportOptions) reportData {
	ic := Icons()
	d := reportData{
		Title:     b.Project.Name,
		Date:      time.Now().Format("02.01.2006"),
		Author:    opt.Author,
		AutoPrint: opt.AutoPrint,
		Project:   b.Project,
	}
	if b.Name != "" {
		d.Title = b.Project.Name + " — " + b.Name
		if t, err := time.Parse(time.RFC3339, b.TakenAt); err == nil {
			d.Date = t.Format("02.01.2006")
		}
	}
	css, _ := web.FS.ReadFile("css/report.css")
	d.CSS = template.CSS(css)
	d.Scan = lastLAN(b)
	if t, err := time.Parse(time.RFC3339, d.Scan.FinishedAt); err == nil {
		d.Scan.FinishedAt = t.Local().Format("02.01.2006 15:04")
	}

	online := map[int64]bool{}
	devByID := map[int64]db.Device{}
	for _, v := range b.Devices {
		online[v.ID] = v.Online
		devByID[v.ID] = v
		if v.Online {
			d.NOnline++
		}
	}
	d.NDevices = len(b.Devices)

	surveyBest := map[int64]string{}
	for _, sp := range b.SurveyPoints {
		if len(sp.Readings) > 0 {
			r := sp.Readings[0]
			surveyBest[sp.ID] = fmt.Sprintf("%s %d dBm", nameOr(r.SSID), r.SignalDBM)
		}
		d.Survey = append(d.Survey, reportSurvey{Label: orDefault(sp.Label, fmt.Sprintf("Punct %d", sp.ID)), Readings: sp.Readings})
	}

	for _, m := range b.Maps {
		sc, err := ParseScene(m.SceneJSON)
		if err != nil {
			continue
		}
		fillNodeLabels(sc, b)
		svg := RenderSVG(sc, RenderContext{Online: online, SurveyBest: surveyBest, BackgroundURI: m.Background, Legend: true})
		d.Maps = append(d.Maps, reportMap{Name: m.Name, SVG: template.HTML(svg)})
	}

	// Wi-Fi grupat pe SSID.
	groups := map[string][]reportBSSID{}
	aps := map[string]bool{}
	var analyze []wifi.AnalyzeInput
	for _, w := range b.Wifi {
		rb := reportBSSID{WifiNetwork: w, AP: "neasociat", Signal: "—"}
		if w.SignalDBM != nil {
			rb.Signal = fmt.Sprintf("%d dBm", *w.SignalDBM)
		}
		if w.APDeviceID != nil && w.APLinkStatus != "respins" {
			if dv, ok := devByID[*w.APDeviceID]; ok {
				rb.AP = DisplayName(dv) + " (" + w.APLinkStatus + ")"
			}
		}
		groups[w.SSID] = append(groups[w.SSID], rb)
		// Un echipament = grup de BSSID-uri „pereche”.
		key := w.BSSID
		for k := range aps {
			if wifi.SameRadioPair(k, w.BSSID) {
				key = k
				break
			}
		}
		aps[key] = true
		analyze = append(analyze, wifi.AnalyzeInput{SSID: w.SSID, BSSID: w.BSSID, Band: w.Band, Channel: w.Channel, Signal: w.SignalDBM})
	}
	d.NAPs = len(aps)
	for s, list := range groups {
		d.SSIDGroups = append(d.SSIDGroups, reportSSID{SSID: nameOr(s), BSSIDs: list})
		d.SSIDs = append(d.SSIDs, nameOr(s))
	}
	sort.Slice(d.SSIDGroups, func(i, j int) bool { return d.SSIDGroups[i].SSID < d.SSIDGroups[j].SSID })
	sort.Strings(d.SSIDs)

	// Dispozitive grupate pe locație.
	byLoc := map[string][]reportDevice{}
	unknown := 0
	for _, v := range b.Devices {
		t := EffectiveType(v)
		label := t
		if def, ok := ic[t]; ok {
			label = def.Label
		}
		if t == "necunoscut" && v.CustomName == "" {
			unknown++
		}
		byLoc[v.Location] = append(byLoc[v.Location], reportDevice{Device: v, Name: DisplayName(v), Type: t, TypeLabel: label, Ports: PortsString(v.OpenPorts)})
	}
	locs := make([]string, 0, len(byLoc))
	for l := range byLoc {
		locs = append(locs, l)
	}
	sort.Slice(locs, func(i, j int) bool {
		if locs[i] == "" || locs[j] == "" {
			return locs[j] == ""
		}
		return locs[i] < locs[j]
	})
	for _, l := range locs {
		d.Groups = append(d.Groups, reportGroup{Location: orDefault(l, "Fără locație"), Devices: byLoc[l]})
	}

	// Bluetooth: doar dispozitivele cu nume sau completate de utilizator (restul sunt
	// de obicei telefoane/ceasuri ale trecătorilor, cu adrese aleatoare).
	for _, bt := range b.Bluetooth {
		if bt.Name == "" && bt.CustomName == "" && bt.Location == "" {
			d.BTHidden++
			continue
		}
		t := BluetoothType(bt)
		label := t
		if def, ok := ic[t]; ok {
			label = def.Label
		}
		rb := reportBT{Name: BluetoothName(bt), TypeLabel: label, MAC: bt.MAC, Manufacturer: bt.Manufacturer, Signal: "—",
			Location: bt.Location, Notes: bt.Notes, Visible: bt.Visible}
		if bt.RSSI != nil {
			rb.Signal = fmt.Sprintf("%d dBm", *bt.RSSI)
		}
		if bt.DeviceID != nil && bt.LinkStatus != "respins" {
			if dv, ok := devByID[*bt.DeviceID]; ok {
				rb.Link = DisplayName(dv) + " (" + bt.LinkStatus + ")"
			}
		}
		d.Bluetooth = append(d.Bluetooth, rb)
	}

	d.Warnings = wifi.Analyze(analyze)
	if unknown > 0 {
		d.Warnings = append(d.Warnings, wifi.Warning{Level: "info", Message: fmt.Sprintf("%d dispozitive neidentificate (tip necunoscut, fără nume). Verifică-le și denumește-le în tab-ul Dispozitive.", unknown)})
	}
	offline := 0
	for _, v := range b.Devices {
		if !v.Online {
			offline++
		}
	}
	if offline > 0 && d.Scan.ID != 0 {
		d.Warnings = append(d.Warnings, wifi.Warning{Level: "info", Message: fmt.Sprintf("%d dispozitive din inventar nu au răspuns la ultimul scan (offline sau scoase din rețea).", offline)})
	}
	return d
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// RenderReport întoarce raportul ca șir (pentru API).
func RenderReport(b *Backup, opt ReportOptions) ([]byte, error) {
	var buf bytes.Buffer
	err := WriteHTML(&buf, b, opt)
	return buf.Bytes(), err
}

var reportTpl = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html lang="ro">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Raport rețea — {{.Title}}</title>
<style>{{.CSS}}</style>
</head>
<body>
<div class="toolbar"><button onclick="window.print()">Salvează ca PDF / Tipărește</button></div>
<section class="cover page">
  <div class="brand">webrt-network-mapper</div>
  <h1>{{.Project.Name}}</h1>
  <p class="sub">Raport de inventar rețea</p>
  <dl>
    {{if .Project.Address}}<dt>Adresă</dt><dd>{{.Project.Address}}</dd>{{end}}
    <dt>Data</dt><dd>{{.Date}}</dd>
    {{if .Author}}<dt>Autor</dt><dd>{{.Author}}</dd>{{end}}
  </dl>
  {{if .Project.Notes}}<p class="notes">{{.Project.Notes}}</p>{{end}}
</section>

<section class="page">
  <h2>1. Rezumat</h2>
  <div class="stats">
    <div><b>{{.NDevices}}</b><span>dispozitive în inventar</span></div>
    <div><b>{{.NOnline}}</b><span>online la ultimul scan</span></div>
    <div><b>{{.NAPs}}</b><span>echipamente Wi-Fi (estimat)</span></div>
    <div><b>{{len .SSIDs}}</b><span>SSID-uri detectate</span></div>
  </div>
  <table class="kv">
    <tr><th>Subnet</th><td>{{or .Scan.Subnet "—"}}</td></tr>
    <tr><th>Gateway</th><td>{{or .Scan.Gateway "—"}}</td></tr>
    <tr><th>DNS</th><td>{{or .Scan.DNS "—"}}</td></tr>
    <tr><th>Interfață scan</th><td>{{or .Scan.Interface "—"}} {{if .Scan.Method}}({{.Scan.Method}}){{end}}</td></tr>
    <tr><th>Ultimul scan</th><td>{{or .Scan.FinishedAt "—"}}</td></tr>
    <tr><th>SSID-uri</th><td>{{range $i, $s := .SSIDs}}{{if $i}}, {{end}}{{$s}}{{else}}—{{end}}</td></tr>
  </table>
</section>

{{range .Maps}}
<section class="page landscape map">
  <h2>2. Harta locației — {{.Name}}</h2>
  <div class="svgwrap">{{.SVG}}</div>
</section>
{{end}}

<section class="page">
  <h2>3. Infrastructură wireless</h2>
  {{range .SSIDGroups}}
  <h3>{{.SSID}} <small>{{len .BSSIDs}} BSSID-uri</small></h3>
  <table>
    <thead><tr><th>BSSID</th><th>Bandă</th><th>Canal</th><th>Semnal</th><th>Securitate</th><th>Etichetă</th><th>Locație</th><th>Echipament (AP)</th></tr></thead>
    <tbody>
    {{range .BSSIDs}}<tr><td class="mono">{{.BSSID}}</td><td>{{.Band}} GHz</td><td>{{.Channel}}</td><td>{{.Signal}}</td><td>{{.Security}}</td><td>{{.Label}}</td><td>{{.Location}}</td><td>{{.AP}}</td></tr>{{end}}
    </tbody>
  </table>
  {{else}}<p class="muted">Nu a fost rulat niciun scan Wi-Fi.</p>{{end}}
</section>

<section class="page">
  <h2>4. Dispozitive pe locații</h2>
  {{range .Groups}}
  <h3>{{.Location}} <small>{{len .Devices}}</small></h3>
  <table>
    <thead><tr><th></th><th>Nume</th><th>Tip</th><th>IP</th><th>MAC</th><th>Vendor</th><th>Porturi</th><th>Observații</th></tr></thead>
    <tbody>
    {{range .Devices}}<tr><td><span class="dot {{if .Online}}on{{end}}"></span></td><td><b>{{.Name}}</b>{{if and .Hostname (ne .Hostname .Name)}}<br><small>{{.Hostname}}</small>{{end}}</td><td>{{.TypeLabel}}</td><td class="mono">{{.LastIP}}</td><td class="mono">{{.MAC}}</td><td>{{.Vendor}}</td><td>{{.Ports}}</td><td>{{.Notes}}</td></tr>{{end}}
    </tbody>
  </table>
  {{else}}<p class="muted">Inventarul este gol.</p>{{end}}
</section>

{{if or .Bluetooth .BTHidden}}
<section class="page">
  <h2>Bluetooth în apropiere</h2>
  <p class="muted">Descoperire pasivă (doar anunțurile emise de dispozitive, fără conectare).{{if .BTHidden}} {{.BTHidden}} dispozitive anonime (fără nume, de obicei telefoane/ceasuri din apropiere) nu sunt listate.{{end}}</p>
  {{if .Bluetooth}}
  <table>
    <thead><tr><th></th><th>Nume</th><th>Tip</th><th>MAC</th><th>Producător</th><th>Semnal</th><th>Același cu (LAN)</th><th>Locație</th><th>Observații</th></tr></thead>
    <tbody>{{range .Bluetooth}}<tr><td><span class="dot {{if .Visible}}on{{end}}"></span></td><td><b>{{.Name}}</b></td><td>{{.TypeLabel}}</td><td class="mono">{{.MAC}}</td><td>{{.Manufacturer}}</td><td>{{.Signal}}</td><td>{{.Link}}</td><td>{{.Location}}</td><td>{{.Notes}}</td></tr>{{end}}</tbody>
  </table>
  {{end}}
</section>
{{end}}

<section class="page">
  <h2>5. Observații și recomandări</h2>
  {{range .Warnings}}<p class="warn {{.Level}}">{{.Message}}</p>{{else}}<p class="muted">Nu au fost detectate probleme.</p>{{end}}
</section>

{{if .Survey}}
<section class="page">
  <h2>Anexă — rezultate survey Wi-Fi</h2>
  {{range .Survey}}
  <h3>{{.Label}}</h3>
  <table>
    <thead><tr><th>SSID</th><th>BSSID</th><th>Bandă</th><th>Canal</th><th>Semnal</th></tr></thead>
    <tbody>{{range .Readings}}<tr><td>{{.SSID}}</td><td class="mono">{{.BSSID}}</td><td>{{.Band}} GHz</td><td>{{.Channel}}</td><td>{{.SignalDBM}} dBm</td></tr>{{end}}</tbody>
  </table>
  {{end}}
</section>
{{end}}
{{if .AutoPrint}}<script>window.addEventListener('load', () => setTimeout(() => window.print(), 300));</script>{{end}}
</body>
</html>`))

func nameOr(s string) string {
	if s == "" {
		return "<SSID ascuns>"
	}
	return s
}

// RenderMaps randează toate hărțile unui backup ca SVG (nume hartă → SVG).
func RenderMaps(b *Backup, legend bool) map[string]string {
	online := map[int64]bool{}
	for _, v := range b.Devices {
		online[v.ID] = v.Online
	}
	surveyBest := map[int64]string{}
	for _, sp := range b.SurveyPoints {
		if len(sp.Readings) > 0 {
			surveyBest[sp.ID] = fmt.Sprintf("%s %d dBm", nameOr(sp.Readings[0].SSID), sp.Readings[0].SignalDBM)
		}
	}
	out := map[string]string{}
	for _, m := range b.Maps {
		sc, err := ParseScene(m.SceneJSON)
		if err != nil {
			continue
		}
		fillNodeLabels(sc, b)
		out[m.Name] = RenderSVG(sc, RenderContext{Online: online, SurveyBest: surveyBest, BackgroundURI: m.Background, Legend: legend})
	}
	return out
}

// fillNodeLabels completează etichetele goale ale nodurilor din inventar
// (în editor, eticheta implicită e numele dispozitivului).
func fillNodeLabels(sc *Scene, b *Backup) {
	devs := map[int64]db.Device{}
	for _, v := range b.Devices {
		devs[v.ID] = v
	}
	bts := map[int64]db.BluetoothDevice{}
	for _, v := range b.Bluetooth {
		bts[v.ID] = v
	}
	for i := range sc.Nodes {
		n := &sc.Nodes[i]
		if n.BtID != nil && n.Label == "" {
			if bt, ok := bts[*n.BtID]; ok {
				n.Label = BluetoothName(bt)
			}
		}
		if n.DeviceID == nil {
			continue
		}
		d, ok := devs[*n.DeviceID]
		if !ok {
			continue
		}
		if n.Label == "" {
			n.Label = DisplayName(d)
		}
		if n.Sublabel == "" && d.LastIP != "" && d.LastIP != n.Label {
			n.Sublabel = d.LastIP
		}
		if n.Color == "" {
			n.Color = d.Color
		}
	}
}

// BluetoothName: numele dat de utilizator, apoi cel anunțat, apoi MAC-ul.
func BluetoothName(b db.BluetoothDevice) string {
	switch {
	case b.CustomName != "":
		return b.CustomName
	case b.Name != "":
		return b.Name
	}
	return b.MAC
}

// BluetoothType: tipul ales de utilizator suprascrie tipul ghicit.
func BluetoothType(b db.BluetoothDevice) string {
	if b.DeviceType != "" {
		return b.DeviceType
	}
	if b.DetectedType != "" {
		return b.DetectedType
	}
	return "bluetooth"
}
