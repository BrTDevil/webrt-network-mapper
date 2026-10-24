// Package config ține setările aplicației.
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// AppName este numele binarului și al directorului de date.
const AppName = "webrt-network-mapper"

type Config struct {
	Addr    string // adresa serverului web, mereu pe 127.0.0.1
	DataDir string
	DBPath  string
}

// Default returnează configurația implicită.
//
// Aplicația e instalată global (ex. /usr/local/bin), deci datele stau în
// directorul utilizatorului, nu lângă binar:
//   - Linux:   $XDG_DATA_HOME/webrt-network-mapper sau ~/.local/share/webrt-network-mapper
//   - macOS:   ~/Library/Application Support/webrt-network-mapper
//   - Windows: %AppData%\webrt-network-mapper
//
// Variabila NETMAP_DATA suprascrie directorul.
func Default() Config {
	dir := os.Getenv("NETMAP_DATA")
	if dir == "" {
		dir = defaultDataDir()
	}
	return Config{
		Addr:    "127.0.0.1:8765",
		DataDir: dir,
		DBPath:  filepath.Join(dir, "network-map.db"),
	}
}

func defaultDataDir() string {
	// sudo păstrează HOME-ul utilizatorului real doar cu anumite setări;
	// folosim SUDO_USER ca să nu scriem în /root din greșeală.
	home, _ := os.UserHomeDir()
	if u := os.Getenv("SUDO_USER"); u != "" && runtime.GOOS != "windows" {
		if runtime.GOOS == "darwin" {
			home = filepath.Join("/Users", u)
		} else {
			home = filepath.Join("/home", u)
		}
	}
	switch runtime.GOOS {
	case "windows":
		if d, err := os.UserConfigDir(); err == nil {
			return filepath.Join(d, AppName)
		}
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", AppName)
	default:
		if x := os.Getenv("XDG_DATA_HOME"); x != "" && os.Getenv("SUDO_USER") == "" {
			return filepath.Join(x, AppName)
		}
		return filepath.Join(home, ".local", "share", AppName)
	}
	return filepath.Join(home, "."+AppName)
}
