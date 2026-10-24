//go:build !windows

package main

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// fixOwnership: rulat cu sudo, fișierele create în home-ul utilizatorului ar
// aparține lui root; le dăm înapoi utilizatorului real.
func fixOwnership(dir string) {
	name := os.Getenv("SUDO_USER")
	if name == "" || os.Geteuid() != 0 {
		return
	}
	u, err := user.Lookup(name)
	if err != nil {
		return
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	filepath.Walk(dir, func(p string, _ os.FileInfo, err error) error {
		if err == nil {
			os.Lchown(p, uid, gid)
		}
		return nil
	})
	// Și folderele părinte create de noi (ex. ~/.local/share).
	for p := filepath.Dir(dir); strings.HasPrefix(p, u.HomeDir+string(os.PathSeparator)); p = filepath.Dir(p) {
		if st, err := os.Stat(p); err == nil {
			if sys, ok := statOwner(st); ok && sys == 0 {
				os.Lchown(p, uid, gid)
			}
		}
	}
}

func rawARPStatus() string {
	if os.Geteuid() == 0 {
		return "disponibil (root)"
	}
	exe, err := os.Executable()
	if err != nil {
		return "necunoscut"
	}
	out, err := exec.Command("getcap", exe).Output()
	if err == nil && strings.Contains(string(out), "cap_net_raw") {
		return "disponibil (cap_net_raw)"
	}
	return "indisponibil — se folosește ping + tabela ARP (vezi README: setcap)"
}
