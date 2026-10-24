// Package snapshot salvează instantanee ale unui proiect ca fișiere .netmap.json
// într-un folder ales de utilizator, le redenumește și le compară.
package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"webrt-network-mapper/internal/db"
	"webrt-network-mapper/internal/export"
)

// Ext e extensia fișierelor de instantaneu.
const Ext = ".netmap.json"

// SettingKey e cheia din tabela settings pentru folderul instantaneelor.
const SettingKey = "snapshot_dir"

// Info descrie un instantaneu de pe disc.
type Info struct {
	Path        string `json:"path"` // relativ la folderul instantaneelor, cu „/”
	Name        string `json:"name"` // numele fișierului fără extensie
	Folder      string `json:"folder"`
	ProjectName string `json:"project_name"`
	ProjectID   int64  `json:"project_id"`
	TakenAt     string `json:"taken_at"`
	Note        string `json:"note"`
	Devices     int    `json:"devices"`
	Wifi        int    `json:"wifi"`
	Maps        int    `json:"maps"`
	Size        int64  `json:"size"`
}

// DefaultDir: ~/Documente/webrt-network-mapper (sau ~/Documents/… dacă nu există „Documente”).
func DefaultDir() string {
	home, _ := os.UserHomeDir()
	if u := os.Getenv("SUDO_USER"); u != "" {
		home = filepath.Join(filepath.Dir(home), u)
		if _, err := os.Stat(home); err != nil {
			home, _ = os.UserHomeDir()
		}
	}
	for _, d := range []string{"Documente", "Documents"} {
		if st, err := os.Stat(filepath.Join(home, d)); err == nil && st.IsDir() {
			return filepath.Join(home, d, "webrt-network-mapper")
		}
	}
	return filepath.Join(home, "webrt-network-mapper")
}

// Dir întoarce folderul configurat (sau cel implicit).
func Dir(d *db.DB) string {
	if v := d.GetSetting(SettingKey); v != "" {
		return v
	}
	return DefaultDir()
}

// SetDir validează și salvează folderul instantaneelor.
func SetDir(d *db.DB, dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", errors.New("folderul nu poate fi gol")
	}
	if strings.HasPrefix(dir, "~") {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("nu pot crea folderul: %w", err)
	}
	return abs, d.SetSetting(SettingKey, abs)
}

var reBadChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// SafeName curăță un nume ca să poată fi folosit ca nume de fișier.
func SafeName(name string) string {
	name = reBadChars.ReplaceAllString(strings.TrimSpace(name), "-")
	name = strings.Trim(name, ". ")
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
	}
	return name
}

// resolve transformă o cale relativă primită din UI într-o cale absolută sigură,
// garantat în interiorul folderului instantaneelor.
func resolve(root, rel string) (string, error) {
	rel = filepath.FromSlash(rel)
	if filepath.IsAbs(rel) || rel == "" {
		return "", errors.New("cale invalidă")
	}
	p := filepath.Join(root, rel)
	r, err := filepath.Rel(root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", errors.New("cale în afara folderului de instantanee")
	}
	return p, nil
}

// Save scrie un instantaneu nou. folder e opțional (subfolder, ex. numele locației).
func Save(d *db.DB, projectID int64, name, note, folder string) (*Info, error) {
	b, err := export.BuildBackup(d, projectID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if strings.TrimSpace(name) == "" {
		name = now.Format("2006-01-02 1504") + " - " + b.Project.Name
	}
	name = SafeName(name)
	if name == "" {
		return nil, errors.New("nume invalid")
	}
	b.Name = name
	b.Note = note
	b.TakenAt = now.Format(time.RFC3339)

	root := Dir(d)
	dir := root
	if f := SafeName(folder); f != "" {
		dir = filepath.Join(root, f)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("nu pot crea folderul %s: %w", dir, err)
	}
	path := uniquePath(filepath.Join(dir, name+Ext))
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(path, data); err != nil {
		return nil, err
	}
	return stat(root, path)
}

// List întoarce toate instantaneele din folder (inclusiv subfoldere), cele mai noi primele.
func List(d *db.DB) ([]Info, error) {
	root := Dir(d)
	out := []Info{}
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil // ignoră foldere inaccesibile
		}
		if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) {
			return nil
		}
		if info, err := stat(root, p); err == nil {
			out = append(out, *info)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].TakenAt > out[j].TakenAt })
	return out, err
}

// Load citește un instantaneu după calea relativă.
func Load(d *db.DB, rel string) (*export.Backup, error) {
	p, err := resolve(Dir(d), rel)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	b, err := export.ParseBackup(data)
	if err != nil {
		return nil, err
	}
	b.Name = strings.TrimSuffix(filepath.Base(p), Ext)
	return b, nil
}

// Rename redenumește fișierul (și numele din interior). Întoarce noua cale relativă.
func Rename(d *db.DB, rel, newName string) (*Info, error) {
	root := Dir(d)
	p, err := resolve(root, rel)
	if err != nil {
		return nil, err
	}
	newName = SafeName(newName)
	if newName == "" {
		return nil, errors.New("nume invalid")
	}
	np := filepath.Join(filepath.Dir(p), newName+Ext)
	if np != p {
		if _, err := os.Stat(np); err == nil {
			return nil, errors.New("există deja un instantaneu cu acest nume")
		}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	raw["name"] = newName
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(np, out); err != nil {
		return nil, err
	}
	if np != p {
		if err := os.Remove(p); err != nil {
			return nil, err
		}
	}
	return stat(root, np)
}

// Delete șterge fișierul unui instantaneu (UI-ul cere confirmare).
func Delete(d *db.DB, rel string) error {
	p, err := resolve(Dir(d), rel)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(p, Ext) {
		return errors.New("nu este un fișier de instantaneu")
	}
	return os.Remove(p)
}

// AbsPath întoarce calea absolută (pentru afișare / „deschide folderul”).
func AbsPath(d *db.DB, rel string) (string, error) {
	return resolve(Dir(d), rel)
}

func stat(root, p string) (*Info, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var b export.Backup
	if err := json.Unmarshal(data, &b); err != nil || b.Format != export.Format {
		return nil, errors.New("fișier invalid")
	}
	rel, _ := filepath.Rel(root, p)
	folder := filepath.ToSlash(filepath.Dir(rel))
	if folder == "." {
		folder = ""
	}
	return &Info{
		Path:        filepath.ToSlash(rel),
		Name:        strings.TrimSuffix(filepath.Base(p), Ext),
		Folder:      folder,
		ProjectName: b.Project.Name,
		ProjectID:   b.Project.ID,
		TakenAt:     b.TakenAt,
		Note:        b.Note,
		Devices:     len(b.Devices),
		Wifi:        len(b.Wifi),
		Maps:        len(b.Maps),
		Size:        st.Size(),
	}, nil
}

func uniquePath(p string) string {
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		return p
	}
	base := strings.TrimSuffix(p, Ext)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, Ext)
		if _, err := os.Stat(c); errors.Is(err, fs.ErrNotExist) {
			return c
		}
	}
}

func writeAtomic(p string, data []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
