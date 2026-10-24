// Comanda webrt-network-mapper: inventar rețea LAN + Wi-Fi și editor de hartă a locației.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"time"

	"webrt-network-mapper/internal/api"
	"webrt-network-mapper/internal/config"
	"webrt-network-mapper/internal/db"
	"webrt-network-mapper/internal/export"
	"webrt-network-mapper/internal/jobs"
	"webrt-network-mapper/internal/scanner"
	"webrt-network-mapper/internal/snapshot"
)

const usage = `webrt-network-mapper — inventar rețea (LAN + Wi-Fi) și hartă a locației

Utilizare:
  webrt-network-mapper web      [--addr 127.0.0.1:8765] [--no-browser]
  webrt-network-mapper scan     [--lan] [--wifi] [--bt] [--project <id|nume>] [--ssid-filter "RP"]
                                [--connect <SSID>] [--iface wlan0] [--subnet 192.168.1.0/24] [--yes] [--other-network]
  webrt-network-mapper export   --format csv|json|html --project <id|nume> [--out fișier]
  webrt-network-mapper import   <backup.netmap.json> [--name "Nume nou"]
  webrt-network-mapper snapshot save --project <id|nume> [--name "Martie 2026"] [--note "..."]
  webrt-network-mapper snapshot list
  webrt-network-mapper projects
  webrt-network-mapper info
  webrt-network-mapper version

Fără argumente pornește interfața web (echivalent cu „web”).

Date: %s
Variabila NETMAP_DATA schimbă folderul de date.
`

func main() {
	cfg := config.Default()
	args := os.Args[1:]
	cmd := "web"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "web", "serve":
		err = cmdWeb(cfg, args)
	case "scan":
		err = cmdScan(cfg, args)
	case "export":
		err = cmdExport(cfg, args)
	case "import":
		err = cmdImport(cfg, args)
	case "snapshot", "snapshots":
		err = cmdSnapshot(cfg, args)
	case "projects":
		err = cmdProjects(cfg)
	case "info":
		err = cmdInfo(cfg)
	case "version", "--version", "-v":
		fmt.Println(config.AppName, api.Version)
	case "help", "-h", "--help":
		fmt.Printf(usage, cfg.DataDir)
	default:
		fmt.Fprintf(os.Stderr, "comandă necunoscută: %s\n\n", cmd)
		fmt.Fprintf(os.Stderr, usage, cfg.DataDir)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "eroare:", err)
		os.Exit(1)
	}
}

func openDB(cfg config.Config) (*db.DB, error) {
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("baza de date %s: %w", cfg.DBPath, err)
	}
	fixOwnership(cfg.DataDir)
	return d, nil
}

// ---------- web ----------

func cmdWeb(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	addr := fs.String("addr", cfg.Addr, "adresa de ascultare (doar 127.0.0.1)")
	noBrowser := fs.Bool("no-browser", false, "nu deschide browserul")
	fs.Parse(args)

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && host != "localhost" {
		return errors.New("serverul ascultă doar pe 127.0.0.1 (aplicație locală)")
	}
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		// Portul e ocupat: poate aplicația rulează deja — deschidem doar browserul.
		if isOurServer(*addr) {
			fmt.Println("Aplicația rulează deja pe http://" + *addr)
			if !*noBrowser {
				openBrowser("http://" + *addr)
			}
			return nil
		}
		return fmt.Errorf("nu pot asculta pe %s: %w", *addr, err)
	}
	srv := &api.Server{DB: d, Runner: jobs.NewRunner(d), Cfg: cfg}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	url := "http://" + *addr
	fmt.Printf("webrt-network-mapper %s\n", api.Version)
	fmt.Printf("  Interfață:  %s\n", url)
	fmt.Printf("  Date:       %s\n", cfg.DBPath)
	fmt.Printf("  Instantanee: %s\n", snapshot.Dir(d))
	fmt.Println("Ctrl+C pentru oprire.")
	if !*noBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		httpSrv.Shutdown(sh)
	}()
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	fmt.Println("\nOprit.")
	return nil
}

func isOurServer(addr string) bool {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get("http://" + addr + "/api/info")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return strings.Contains(string(b), `"snapshot_dir"`)
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		// Sub sudo, browserul trebuie pornit ca utilizatorul real.
		if u := os.Getenv("SUDO_USER"); u != "" {
			c = exec.Command("sudo", "-u", u, "xdg-open", url)
		} else {
			c = exec.Command("xdg-open", url)
		}
	}
	c.Stdout, c.Stderr = nil, nil
	if err := c.Start(); err == nil {
		go c.Wait()
	}
}

// ---------- scan ----------

func cmdScan(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	lan := fs.Bool("lan", false, "doar LAN")
	wifiF := fs.Bool("wifi", false, "doar Wi-Fi")
	proj := fs.String("project", "", "ID-ul sau numele proiectului (implicit: primul / „Rețeaua mea”)")
	iface := fs.String("iface", "", "interfața de rețea")
	subnet := fs.String("subnet", "", "subnet explicit (ex. 192.168.1.0/24)")
	yes := fs.Bool("yes", false, "confirmă scanarea (utilizare legitimă, subnet mare)")
	bt := fs.Bool("bt", false, "Bluetooth (pasiv, ~12 s)")
	ssidFilter := fs.String("ssid-filter", "", "Wi-Fi: doar rețelele care conțin acești termeni (ex. \"RP\"); \"-\" = șterge filtrul")
	connect := fs.String("connect", "", "scanează LAN-ul altei rețele Wi-Fi (parola se cere ascuns; nu se salvează)")
	otherNet := fs.Bool("other-network", false, "scanează în proiect chiar dacă routerul rețelei curente nu a mai fost văzut în el")
	fs.Parse(args)
	if !*lan && !*wifiF && !*bt {
		*lan, *wifiF = true, true
	}
	if *connect != "" {
		*lan = true
	}
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()

	if !*yes && d.GetSetting("scan_consent") != "1" {
		fmt.Println("Folosește aplicația doar pe rețele pe care le administrezi sau pentru care ai acordul proprietarului.")
		fmt.Print("Confirmi? [d/N] ")
		ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		ans = strings.ToLower(strings.TrimSpace(ans))
		if ans != "d" && ans != "da" && ans != "y" && ans != "yes" {
			return errors.New("scan anulat")
		}
		d.SetSetting("scan_consent", "1")
	}

	p, err := resolveProject(d, *proj, true)
	if err != nil {
		return err
	}
	fmt.Printf("Proiect: %s (#%d)\n", p.Name, p.ID)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	r := jobs.NewRunner(d)
	// Altă rețea decât cele scanate până acum în proiect → nu amestecăm locațiile.
	if !*otherNet && *connect == "" {
		var other *jobs.OtherNetworkError
		if err := r.CheckNetwork(ctx, p.ID, *iface); errors.As(err, &other) {
			hint := fmt.Sprintf("--project \"Nume locație nouă\" (se creează automat)")
			if len(other.Matches) > 0 {
				hint = fmt.Sprintf("--project %d (%s)", other.Matches[0].ID, other.Matches[0].Name)
			}
			return fmt.Errorf("%w.\nFolosește %s, sau --other-network dacă e aceeași locație (ex. un al doilea router)", err, hint)
		}
	}
	if *ssidFilter != "" {
		f := *ssidFilter
		if f == "-" {
			f = ""
		}
		d.SetSetting(jobs.SSIDFilterKey(p.ID), f)
	}
	var password string
	if *connect != "" {
		fmt.Printf("Parola rețelei %q (Enter = folosește profilul salvat în sistem): ", *connect)
		password = readPassword()
		fmt.Println()
	}
	var btDone chan struct{}
	if *bt {
		btDone = make(chan struct{})
		go func() {
			defer close(btDone)
			id, _ := d.CreateScan(p.ID, "bluetooth")
			if err := r.RunBluetooth(ctx, p.ID, id); err != nil {
				fmt.Fprintln(os.Stderr, "\nBluetooth:", err)
			}
		}()
	}

	if *lan {
		id, _ := d.CreateScan(p.ID, "lan")
		done := make(chan struct{})
		go showProgress(d, id, done)
		err := r.RunLANConnected(ctx, p.ID, id, jobs.Request{Interface: *iface, Subnet: *subnet, AllowLarge: *yes, ConnectSSID: *connect, Password: password})
		password = ""
		close(done)
		var big *scanner.ErrSubnetTooLarge
		if errors.As(err, &big) {
			return fmt.Errorf("%w — adaugă --yes pentru a continua", err)
		}
		if err != nil {
			return err
		}
		sc, _ := d.GetScan(id)
		fmt.Printf("\rLAN: %s. Subnet %s, gateway %s\n", sc.Message, sc.Subnet, sc.Gateway)
		devs, _ := d.ListDevices(p.ID)
		fmt.Printf("\n%-16s %-18s %-12s %-22s %s\n", "IP", "MAC", "TIP", "VENDOR", "NUME / HOSTNAME")
		for _, v := range devs {
			if !v.Online {
				continue
			}
			fmt.Printf("%-16s %-18s %-12s %-22s %s\n", v.LastIP, v.MAC, export.EffectiveType(v), trunc(v.Vendor, 22), export.DisplayName(v))
		}
		fmt.Println()
	}
	if *wifiF {
		id, _ := d.CreateScan(p.ID, "wifi")
		if err := r.RunWiFi(ctx, p.ID, id, nil); err != nil {
			fmt.Fprintln(os.Stderr, "Wi-Fi:", err)
		} else {
			sc, _ := d.GetScan(id)
			fmt.Println("Wi-Fi:", sc.Message)
			nets, _ := d.ListWifi(p.ID)
			fmt.Printf("\n%-24s %-18s %-5s %-4s %s\n", "SSID", "BSSID", "BANDĂ", "CH", "SEMNAL")
			for _, n := range nets {
				if !n.Visible {
					continue
				}
				sig := ""
				if n.SignalDBM != nil {
					sig = strconv.Itoa(*n.SignalDBM) + " dBm"
				}
				fmt.Printf("%-24s %-18s %-5s %-4d %s\n", trunc(orHidden(n.SSID), 24), n.BSSID, n.Band, n.Channel, sig)
			}
		}
	}
	if btDone != nil {
		<-btDone
		list, _ := d.ListBluetooth(p.ID)
		fmt.Printf("\n%-28s %-18s %-20s %s\n", "BLUETOOTH (cu nume)", "MAC", "TIP", "SEMNAL")
		for _, b := range list {
			if !b.Visible || b.Name == "" {
				continue
			}
			sig := ""
			if b.RSSI != nil {
				sig = strconv.Itoa(*b.RSSI) + " dBm"
			}
			fmt.Printf("%-28s %-18s %-20s %s\n", trunc(b.Name, 28), b.MAC, export.BluetoothType(b), sig)
		}
	}
	fmt.Println("\nRezultatele sunt salvate. Deschide interfața cu: webrt-network-mapper web")
	return nil
}

func showProgress(d *db.DB, id int64, done chan struct{}) {
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			if sc, err := d.GetScan(id); err == nil {
				fmt.Printf("\r\033[K[%3d%%] %s", sc.Progress, trunc(sc.Message, 70))
			}
		}
	}
}

func resolveProject(d *db.DB, ref string, create bool) (*db.Project, error) {
	ps, err := d.ListProjects()
	if err != nil {
		return nil, err
	}
	if ref == "" {
		if len(ps) > 0 {
			return &ps[0], nil
		}
		if !create {
			return nil, errors.New("nu există niciun proiect")
		}
		return d.CreateProject("Rețeaua mea", "", "")
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return d.GetProject(id)
	}
	for _, p := range ps {
		if strings.EqualFold(p.Name, ref) {
			return &p, nil
		}
	}
	if create {
		return d.CreateProject(ref, "", "")
	}
	return nil, fmt.Errorf("proiectul %q nu există", ref)
}

// ---------- export / import ----------

func cmdExport(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	format := fs.String("format", "html", "csv | json | html")
	proj := fs.String("project", "", "ID-ul sau numele proiectului")
	out := fs.String("out", "", "fișier destinație (implicit: nume generat în folderul curent)")
	author := fs.String("author", "", "autorul raportului (html)")
	fs.Parse(args)
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()
	p, err := resolveProject(d, *proj, false)
	if err != nil {
		return err
	}
	date := time.Now().Format("2006-01-02")
	base := snapshot.SafeName(p.Name)
	if *out == "" {
		switch *format {
		case "csv":
			*out = fmt.Sprintf("dispozitive-%s-%s.csv", base, date)
		case "json":
			*out = fmt.Sprintf("backup-%s-%s.netmap.json", base, date)
		case "html":
			*out = fmt.Sprintf("network-map-%s.html", date)
		default:
			return fmt.Errorf("format necunoscut %q", *format)
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	switch *format {
	case "csv":
		devs, err := d.ListDevices(p.ID)
		if err != nil {
			return err
		}
		err = export.WriteCSV(f, devs)
	case "json":
		b, err := export.BuildBackup(d, p.ID)
		if err != nil {
			return err
		}
		enc := jsonEncoder(f)
		err = enc.Encode(b)
	case "html":
		b, err := export.BuildBackup(d, p.ID)
		if err != nil {
			return err
		}
		err = export.WriteHTML(f, b, export.ReportOptions{Author: *author})
	default:
		return fmt.Errorf("format necunoscut %q", *format)
	}
	if err != nil {
		return err
	}
	fmt.Println("Scris:", *out)
	return nil
}

func cmdImport(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	name := fs.String("name", "", "numele noului proiect")
	// Permite fișierul înainte sau după flag-uri.
	var file string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		file, args = args[0], args[1:]
	}
	fs.Parse(args)
	if file == "" {
		file = fs.Arg(0)
	}
	if file == "" {
		return errors.New("specifică fișierul de backup")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	b, err := export.ParseBackup(data)
	if err != nil {
		return err
	}
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()
	pid, err := export.Restore(d, b, *name)
	if err != nil {
		return err
	}
	fmt.Printf("Importat ca proiectul #%d\n", pid)
	return nil
}

// ---------- instantanee ----------

func cmdSnapshot(cfg config.Config, args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()
	switch args[0] {
	case "list", "ls":
		list, err := snapshot.List(d)
		if err != nil {
			return err
		}
		fmt.Println("Folder:", snapshot.Dir(d))
		if len(list) == 0 {
			fmt.Println("Niciun instantaneu salvat.")
		}
		for _, s := range list {
			t, _ := time.Parse(time.RFC3339, s.TakenAt)
			fmt.Printf("  %s  %-40s  %3d dispozitive  %3d Wi-Fi  (%s)\n", t.Format("2006-01-02 15:04"), s.Path, s.Devices, s.Wifi, s.ProjectName)
		}
		return nil
	case "save":
		fs := flag.NewFlagSet("snapshot save", flag.ExitOnError)
		proj := fs.String("project", "", "ID-ul sau numele proiectului")
		name := fs.String("name", "", "numele instantaneului (implicit: data + proiect)")
		note := fs.String("note", "", "observații")
		folder := fs.String("folder", "", "subfolder")
		fs.Parse(args[1:])
		p, err := resolveProject(d, *proj, false)
		if err != nil {
			return err
		}
		info, err := snapshot.Save(d, p.ID, *name, *note, *folder)
		if err != nil {
			return err
		}
		fixOwnership(snapshot.Dir(d))
		fmt.Println("Salvat:", snapshot.Dir(d)+string(os.PathSeparator)+info.Path)
		return nil
	case "dir":
		if len(args) > 1 {
			dir, err := snapshot.SetDir(d, args[1])
			if err != nil {
				return err
			}
			fmt.Println("Folder instantanee:", dir)
			return nil
		}
		fmt.Println(snapshot.Dir(d))
		return nil
	}
	return fmt.Errorf("subcomandă necunoscută %q (list, save, dir)", args[0])
}

func cmdProjects(cfg config.Config) error {
	d, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer d.Close()
	ps, err := d.ListProjects()
	if err != nil {
		return err
	}
	for _, p := range ps {
		fmt.Printf("#%-4d %s\n", p.ID, p.Name)
	}
	if len(ps) == 0 {
		fmt.Println("Niciun proiect. Creează unul din interfață sau rulează „scan”.")
	}
	return nil
}

func cmdInfo(cfg config.Config) error {
	fmt.Println("Versiune:   ", api.Version)
	fmt.Println("Date:       ", cfg.DBPath)
	if d, err := db.Open(cfg.DBPath); err == nil {
		fmt.Println("Instantanee:", snapshot.Dir(d))
		d.Close()
	}
	n, err := scanner.DetectNetwork("")
	if err != nil {
		fmt.Println("Rețea:       ", err)
		return nil
	}
	fmt.Printf("Interfață:   %s (%s, %s)\n", n.Interface, n.IP, n.MAC)
	fmt.Printf("Subnet:      %s\nGateway:     %s\nDNS:         %s\n", n.Prefix, n.Gateway, strings.Join(n.DNS, ", "))
	fmt.Println("ARP raw:    ", rawARPStatus())
	return nil
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func orHidden(s string) string {
	if s == "" {
		return "<ascuns>"
	}
	return s
}
