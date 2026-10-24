// Package api expune REST API-ul local și servește frontend-ul.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"webrt-network-mapper/internal/config"
	"webrt-network-mapper/internal/db"
	"webrt-network-mapper/internal/export"
	"webrt-network-mapper/internal/jobs"
	"webrt-network-mapper/internal/scanner"
	"webrt-network-mapper/internal/snapshot"
	"webrt-network-mapper/internal/wifi"
	"webrt-network-mapper/web"
)

// Version e setată la build (-ldflags "-X webrt-network-mapper/internal/api.Version=…").
var Version = "dev"

// Server ține dependențele handler-elor.
type Server struct {
	DB     *db.DB
	Runner *jobs.Runner
	Cfg    config.Config
}

// Handler construiește router-ul complet.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("GET /api/projects/{id}", s.getProject)
	mux.HandleFunc("PATCH /api/projects/{id}", s.updateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.deleteProject)

	mux.HandleFunc("POST /api/projects/{id}/scan", s.startScan)
	mux.HandleFunc("GET /api/projects/{id}/scans", s.listScans)
	mux.HandleFunc("GET /api/scans/{id}", s.getScan)
	mux.HandleFunc("POST /api/scans/{id}/cancel", s.cancelScan)

	mux.HandleFunc("GET /api/projects/{id}/devices", s.listDevices)
	mux.HandleFunc("POST /api/projects/{id}/devices", s.createDevice)
	mux.HandleFunc("PATCH /api/devices/{id}", s.patchDevice)
	mux.HandleFunc("DELETE /api/devices/{id}", s.deleteDevice)

	mux.HandleFunc("GET /api/projects/{id}/wifi", s.listWifi)
	mux.HandleFunc("PATCH /api/wifi/{id}", s.patchWifi)

	mux.HandleFunc("GET /api/projects/{id}/bluetooth", s.listBluetooth)
	mux.HandleFunc("PATCH /api/bluetooth/{id}", s.patchBluetooth)
	mux.HandleFunc("DELETE /api/bluetooth/{id}", s.deleteBluetooth)

	mux.HandleFunc("GET /api/projects/{id}/survey-points", s.listSurvey)
	mux.HandleFunc("POST /api/projects/{id}/survey-points", s.createSurvey)
	mux.HandleFunc("PATCH /api/survey-points/{id}", s.patchSurvey)
	mux.HandleFunc("DELETE /api/survey-points/{id}", s.deleteSurvey)

	mux.HandleFunc("GET /api/projects/{id}/maps", s.listMaps)
	mux.HandleFunc("POST /api/projects/{id}/maps", s.createMap)
	mux.HandleFunc("GET /api/maps/{id}", s.getMap)
	mux.HandleFunc("PUT /api/maps/{id}", s.saveMap)
	mux.HandleFunc("PATCH /api/maps/{id}", s.renameMap)
	mux.HandleFunc("DELETE /api/maps/{id}", s.deleteMap)

	mux.HandleFunc("GET /api/projects/{id}/export", s.exportProject)
	mux.HandleFunc("POST /api/import", s.importBackup)
	mux.HandleFunc("POST /api/render/svg", s.renderSVG)

	mux.HandleFunc("GET /api/settings/snapshot-dir", s.getSnapshotDir)
	mux.HandleFunc("PUT /api/settings/snapshot-dir", s.setSnapshotDir)
	mux.HandleFunc("GET /api/snapshots", s.listSnapshots)
	mux.HandleFunc("POST /api/projects/{id}/snapshots", s.saveSnapshot)
	mux.HandleFunc("PATCH /api/snapshots", s.renameSnapshot)
	mux.HandleFunc("DELETE /api/snapshots", s.deleteSnapshot)
	mux.HandleFunc("GET /api/snapshots/file", s.downloadSnapshot)
	mux.HandleFunc("GET /api/snapshots/report", s.snapshotReport)
	mux.HandleFunc("POST /api/snapshots/restore", s.restoreSnapshot)
	mux.HandleFunc("POST /api/snapshots/open-folder", s.openSnapshotFolder)
	mux.HandleFunc("POST /api/compare", s.compare)

	static, _ := fs.Sub(web.FS, ".")
	mux.Handle("GET /", noCache(http.FileServerFS(static)))

	return s.guard(mux)
}

// guard respinge cererile care nu vin de pe localhost (protecție DNS rebinding / CSRF).
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "[::1]" && host != "::1" {
			http.Error(w, "host nepermis", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && r.Method != http.MethodGet {
			if !strings.HasPrefix(o, "http://127.0.0.1:") && !strings.HasPrefix(o, "http://localhost:") {
				http.Error(w, "origine nepermisă", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

// ---------- utilitare ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, db.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, db.ErrVersionConflict):
		code = http.StatusConflict
	case errors.Is(err, errBadRequest):
		code = http.StatusBadRequest
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

var errBadRequest = errors.New("cerere invalidă")

func badRequest(msg string) error { return fmt.Errorf("%w: %s", errBadRequest, msg) }

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, badRequest("id invalid")
	}
	return id, nil
}

func readJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 64<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest("JSON invalid: " + err.Error())
	}
	return nil
}

// ---------- info ----------

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"version":      Version,
		"os":           runtime.GOOS,
		"db_path":      s.Cfg.DBPath,
		"snapshot_dir": snapshot.Dir(s.DB),
		"ports":        scanner.CommonPorts,
	}
	if n, err := scanner.DetectNetwork(""); err == nil {
		out["network"] = map[string]any{"interface": n.Interface, "ip": n.IP.String(), "subnet": n.Prefix.String(), "gateway": n.Gateway, "dns": n.DNS, "mac": n.MAC}
	}
	if c := wifi.CurrentConnection(r.Context()); c != nil {
		out["wifi"] = c
	}
	writeJSON(w, 200, out)
}

// ---------- proiecte ----------

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.DB.ListProjects()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, ps)
}

type projectBody struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Notes   string `json:"notes"`
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var b projectBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(b.Name) == "" {
		writeErr(w, badRequest("numele proiectului e obligatoriu"))
		return
	}
	p, err := s.DB.CreateProject(strings.TrimSpace(b.Name), b.Address, b.Notes)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, p)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.DB.GetProject(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	last, _ := s.DB.LastLANScan(id)
	writeJSON(w, 200, map[string]any{"project": p, "last_lan_scan": last, "ssid_filter": s.DB.GetSetting(jobs.SSIDFilterKey(id))})
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b projectBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(b.Name) == "" {
		writeErr(w, badRequest("numele proiectului e obligatoriu"))
		return
	}
	if err := s.DB.UpdateProject(id, strings.TrimSpace(b.Name), b.Address, b.Notes); err != nil {
		writeErr(w, err)
		return
	}
	p, _ := s.DB.GetProject(id)
	writeJSON(w, 200, p)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.DB.DeleteProject(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- scanări ----------

func (s *Server) startScan(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := s.DB.GetProject(id); err != nil {
		writeErr(w, err)
		return
	}
	var req jobs.Request
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	ids, err := s.Runner.Start(id, req)
	var other *jobs.OtherNetworkError
	if errors.As(err, &other) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "other_network", "details": other})
		return
	}
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	writeJSON(w, 202, map[string]any{"scan_ids": ids})
}

func (s *Server) listScans(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.DB.ListScans(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getScan(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	sc, err := s.DB.GetScan(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, sc)
}

func (s *Server) cancelScan(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"cancelled": s.Runner.Cancel(id)})
}

// ---------- dispozitive ----------

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.DB.ListDevices(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b struct {
		MAC, IP, Name, Type string
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	mac := scanner.NormalizeMAC(b.MAC)
	if mac == "" {
		// Dispozitive fără MAC cunoscut (ex. switch nemanaged): cheie sintetică unică.
		mac = fmt.Sprintf("MANUAL-%d", time.Now().UnixNano())
	}
	did, err := s.DB.CreateManualDevice(id, mac, b.IP, b.Name, b.Type)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, badRequest("există deja un dispozitiv cu acest MAC"))
			return
		}
		writeErr(w, err)
		return
	}
	d, _ := s.DB.GetDevice(did)
	writeJSON(w, 201, d)
}

func (s *Server) patchDevice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var fields map[string]any
	if err := readJSON(r, &fields); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.DB.PatchDevice(id, fields); err != nil {
		writeErr(w, err)
		return
	}
	d, _ := s.DB.GetDevice(id)
	writeJSON(w, 200, d)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.DB.DeleteDevice(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- Wi-Fi ----------

func (s *Server) listWifi(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.DB.ListWifi(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) patchWifi(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var fields map[string]any
	if err := readJSON(r, &fields); err != nil {
		writeErr(w, err)
		return
	}
	if st, ok := fields["ap_link_status"].(string); ok {
		switch st {
		case "none", "inferat", "confirmat", "respins":
		default:
			writeErr(w, badRequest("status invalid"))
			return
		}
		if st == "respins" || st == "none" {
			fields["ap_device_id"] = nil
		}
	}
	if err := s.DB.PatchWifi(id, fields); err != nil {
		writeErr(w, err)
		return
	}
	n, _ := s.DB.GetWifi(id)
	writeJSON(w, 200, n)
}

// ---------- Bluetooth ----------

func (s *Server) listBluetooth(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.DB.ListBluetooth(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) patchBluetooth(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var fields map[string]any
	if err := readJSON(r, &fields); err != nil {
		writeErr(w, err)
		return
	}
	if st, ok := fields["link_status"].(string); ok {
		switch st {
		case "none", "inferat", "confirmat", "respins":
		default:
			writeErr(w, badRequest("status invalid"))
			return
		}
		if st == "respins" || st == "none" {
			fields["device_id"] = nil
		}
	}
	if err := s.DB.PatchBluetooth(id, fields); err != nil {
		writeErr(w, err)
		return
	}
	b, _ := s.DB.GetBluetooth(id)
	writeJSON(w, 200, b)
}

func (s *Server) deleteBluetooth(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.DB.DeleteBluetooth(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- survey ----------

func (s *Server) listSurvey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.DB.ListSurveyPoints(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

// createSurvey adaugă un punct și rulează imediat un scan Wi-Fi legat de el.
func (s *Server) createSurvey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b struct {
		Label string  `json:"label"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		MapID *int64  `json:"map_id"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	pid, err := s.DB.CreateSurveyPoint(id, b.MapID, b.Label, b.X, b.Y)
	if err != nil {
		writeErr(w, err)
		return
	}
	scanID, err := s.DB.CreateScan(id, "survey")
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	scanErr := s.Runner.RunWiFi(ctx, id, scanID, &pid)
	list, _ := s.DB.ListSurveyPoints(id)
	for _, p := range list {
		if p.ID == pid {
			resp := map[string]any{"point": p}
			if scanErr != nil {
				resp["warning"] = "Scanul Wi-Fi a eșuat: " + scanErr.Error()
			}
			writeJSON(w, 201, resp)
			return
		}
	}
	writeErr(w, db.ErrNotFound)
}

func (s *Server) patchSurvey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var fields map[string]any
	if err := readJSON(r, &fields); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.DB.PatchSurveyPoint(id, fields); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deleteSurvey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.DB.DeleteSurveyPoint(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- hărți ----------

func (s *Server) listMaps(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	list, err := s.DB.ListMaps(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createMap(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b struct {
		Name      string `json:"name"`
		SceneJSON string `json:"scene_json"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(b.Name) == "" {
		writeErr(w, badRequest("numele hărții e obligatoriu"))
		return
	}
	m, err := s.DB.CreateMap(id, strings.TrimSpace(b.Name), b.SceneJSON)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, m)
}

func (s *Server) getMap(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	m, err := s.DB.GetMap(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) saveMap(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b struct {
		SceneJSON  string  `json:"scene_json"`
		Version    int     `json:"version"`
		Name       *string `json:"name"`
		Background *string `json:"background"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if !json.Valid([]byte(b.SceneJSON)) {
		writeErr(w, badRequest("scene_json invalid"))
		return
	}
	v, err := s.DB.SaveMap(id, b.Name, b.SceneJSON, b.Background, b.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]int{"version": v})
}

func (s *Server) renameMap(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(b.Name) == "" {
		writeErr(w, badRequest("numele hărții e obligatoriu"))
		return
	}
	if err := s.DB.RenameMap(id, strings.TrimSpace(b.Name)); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deleteMap(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	m, err := s.DB.GetMap(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	maps, _ := s.DB.ListMaps(m.ProjectID)
	if len(maps) <= 1 {
		writeErr(w, badRequest("proiectul trebuie să aibă cel puțin o hartă"))
		return
	}
	if err := s.DB.DeleteMap(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- export / import ----------

func (s *Server) exportProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	b, err := export.BuildBackup(s.DB, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	date := time.Now().Format("2006-01-02")
	base := snapshot.SafeName(b.Project.Name)
	inline := r.URL.Query().Get("inline") == "1"
	disp := func(name string) {
		if !inline {
			w.Header().Set("Content-Disposition", contentDisposition(name))
		}
	}
	switch r.URL.Query().Get("format") {
	case "csv":
		devs, err := s.DB.ListDevices(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		disp(fmt.Sprintf("dispozitive-%s-%s.csv", base, date))
		export.WriteCSV(w, devs)
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		disp(fmt.Sprintf("backup-%s-%s.netmap.json", base, date))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(b)
	case "html", "pdf":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		disp(fmt.Sprintf("network-map-%s.html", date))
		export.WriteHTML(w, b, export.ReportOptions{Author: r.URL.Query().Get("author"), AutoPrint: r.URL.Query().Get("format") == "pdf"})
	default:
		writeErr(w, badRequest("format necunoscut (csv, json, html, pdf)"))
	}
}

func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r > 127 || r == '"' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, urlEscape(name))
}

func urlEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.IndexByte("-_.~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func (s *Server) importBackup(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<20))
	if err != nil {
		writeErr(w, badRequest("fișier prea mare"))
		return
	}
	b, err := export.ParseBackup(data)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	pid, err := export.Restore(s.DB, b, r.URL.Query().Get("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	p, _ := s.DB.GetProject(pid)
	writeJSON(w, 201, p)
}

// renderSVG randează o scenă (inclusiv nesalvată) în SVG pentru export.
func (s *Server) renderSVG(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SceneJSON  string `json:"scene_json"`
		Background string `json:"background"`
		ProjectID  int64  `json:"project_id"`
		Legend     bool   `json:"legend"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	sc, err := export.ParseScene(b.SceneJSON)
	if err != nil {
		writeErr(w, badRequest("scene_json invalid"))
		return
	}
	rc := export.RenderContext{BackgroundURI: b.Background, Legend: b.Legend, Online: map[int64]bool{}, SurveyBest: map[int64]string{}}
	if b.ProjectID > 0 {
		if devs, err := s.DB.ListDevices(b.ProjectID); err == nil {
			for _, d := range devs {
				rc.Online[d.ID] = d.Online
			}
		}
		if sps, err := s.DB.ListSurveyPoints(b.ProjectID); err == nil {
			for _, sp := range sps {
				if len(sp.Readings) > 0 {
					rc.SurveyBest[sp.ID] = fmt.Sprintf("%s %d dBm", sp.Readings[0].SSID, sp.Readings[0].SignalDBM)
				}
			}
		}
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	io.WriteString(w, export.RenderSVG(sc, rc))
}

// ---------- instantanee ----------

func (s *Server) getSnapshotDir(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"dir": snapshot.Dir(s.DB), "default": snapshot.DefaultDir()})
}

func (s *Server) setSnapshotDir(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Dir string `json:"dir"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	dir, err := snapshot.SetDir(s.DB, b.Dir)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	writeJSON(w, 200, map[string]string{"dir": dir})
}

func (s *Server) listSnapshots(w http.ResponseWriter, r *http.Request) {
	list, err := snapshot.List(s.DB)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"dir": snapshot.Dir(s.DB), "snapshots": list})
}

func (s *Server) saveSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var b struct {
		Name   string `json:"name"`
		Note   string `json:"note"`
		Folder string `json:"folder"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	info, err := snapshot.Save(s.DB, id, b.Name, b.Note, b.Folder)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, info)
}

func (s *Server) renameSnapshot(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	info, err := snapshot.Rename(s.DB, r.URL.Query().Get("path"), b.Name)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	writeJSON(w, 200, info)
}

func (s *Server) deleteSnapshot(w http.ResponseWriter, r *http.Request) {
	if err := snapshot.Delete(s.DB, r.URL.Query().Get("path")); err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) downloadSnapshot(w http.ResponseWriter, r *http.Request) {
	p, err := snapshot.AbsPath(s.DB, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.URL.Query().Get("download") == "1" {
		name := p[strings.LastIndexAny(p, `/\`)+1:]
		w.Header().Set("Content-Disposition", contentDisposition(name))
	}
	http.ServeFile(w, r, p)
}

func (s *Server) snapshotReport(w http.ResponseWriter, r *http.Request) {
	b, err := snapshot.Load(s.DB, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	export.WriteHTML(w, b, export.ReportOptions{AutoPrint: r.URL.Query().Get("print") == "1"})
}

func (s *Server) restoreSnapshot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	b, err := snapshot.Load(s.DB, body.Path)
	if err != nil {
		writeErr(w, badRequest(err.Error()))
		return
	}
	name := body.Name
	if name == "" {
		name = b.Project.Name + " (din " + b.Name + ")"
	}
	pid, err := export.Restore(s.DB, b, name)
	if err != nil {
		writeErr(w, err)
		return
	}
	p, _ := s.DB.GetProject(pid)
	writeJSON(w, 201, p)
}

func (s *Server) openSnapshotFolder(w http.ResponseWriter, r *http.Request) {
	dir := snapshot.Dir(s.DB)
	if _, err := snapshot.SetDir(s.DB, dir); err != nil { // creează folderul dacă lipsește
		writeErr(w, err)
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		writeErr(w, err)
		return
	}
	go cmd.Wait()
	writeJSON(w, 200, map[string]string{"dir": dir})
}

// compare compară două surse: „snap:<cale>” sau „current:<project_id>”.
func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	var body struct {
		A string `json:"a"`
		B string `json:"b"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	load := func(src string) (*export.Backup, string, error) {
		switch {
		case strings.HasPrefix(src, "snap:"):
			b, err := snapshot.Load(s.DB, strings.TrimPrefix(src, "snap:"))
			if err != nil {
				return nil, "", err
			}
			return b, b.Name, nil
		case strings.HasPrefix(src, "current:"):
			id, err := strconv.ParseInt(strings.TrimPrefix(src, "current:"), 10, 64)
			if err != nil {
				return nil, "", err
			}
			b, err := export.BuildBackup(s.DB, id)
			if err != nil {
				return nil, "", err
			}
			return b, "Starea curentă — " + b.Project.Name, nil
		}
		return nil, "", errors.New("sursă invalidă")
	}
	a, la, err := load(body.A)
	if err != nil {
		writeErr(w, badRequest("A: "+err.Error()))
		return
	}
	b, lb, err := load(body.B)
	if err != nil {
		writeErr(w, badRequest("B: "+err.Error()))
		return
	}
	// Ordonare cronologică: A = mai vechi.
	if a.TakenAt > b.TakenAt {
		a, b, la, lb = b, a, lb, la
	}
	diff := snapshot.Compare(a, b, la, lb)
	writeJSON(w, 200, struct {
		*snapshot.Diff
		SvgA map[string]string `json:"svg_a"`
		SvgB map[string]string `json:"svg_b"`
	}{diff, export.RenderMaps(a, false), export.RenderMaps(b, false)})
}
