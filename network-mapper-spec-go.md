# Local Network Mapper — Specificație aplicație

> Unealtă locală (fără cloud) pentru inventarierea unei rețele (LAN + Wi-Fi) și desenarea hărții fizice a locației (restaurant, birou, magazin) într-un editor canvas, cu export în PDF / PNG / SVG / CSV / JSON.

---

## 0. Instrucțiuni pentru agentul care construiește aplicația

- Citește tot documentul înainte de a scrie cod.
- Construiește pe faze (secțiunea 12). La finalul fiecărei faze aplicația trebuie să ruleze.
- Nu șterge fișiere, baze de date sau înregistrări fără confirmarea explicită a utilizatorului.
- Nu rula `git commit` / `git push` fără confirmarea explicită a utilizatorului.
- Frontend: HTML + CSS + JavaScript vanilla, fără framework și fără build step.
- Toate icon-urile sunt SVG proprii, desenate inline în cod. Fără CDN-uri sau resurse externe.
- Interfața este în limba română.

---

## 1. Scop

1. **LAN Discovery** — descoperă dispozitivele active din rețeaua locală (IP, MAC, vendor, hostname, porturi, servicii).
2. **Wireless Survey** — inventariază rețelele Wi-Fi vizibile (SSID, BSSID, canal, bandă, semnal, securitate) și grupează BSSID-urile care emit același SSID (ex. `RP` / `RP_5G` emise de mai multe routere folosite ca amplificatoare, fără mesh).
3. **Editor hartă locație (canvas)** — utilizatorul desenează planul locației: pereți, camere, echipamente (terminal, casă de marcat, server, router, AP etc.) și cablurile dintre ele, cu linii drepte și snap la grilă.
4. **Inventar persistent** — nume, locație și observații date manual, legate de MAC (cheie stabilă; IP-ul se poate schimba prin DHCP).
5. **Export** — raport PDF, hartă PNG/SVG, tabel CSV, backup JSON, HTML self-contained.

---

## 2. Limitări și utilizare legitimă

- Aplicația se folosește doar pe rețele pe care utilizatorul le administrează sau pentru care are acordul proprietarului. La primul scan, afișează un mesaj de confirmare în acest sens.
- Nu cere și nu stochează parole Wi-Fi.
- Nu încearcă autentificări pe dispozitive, nu face brute-force, nu exploatează vulnerabilități. Doar descoperire pasivă/ușoară: ARP, ping, rezolvare hostname, conectare TCP pe o listă scurtă de porturi comune.
- Topologia fizică reală **nu** poate fi dedusă complet dintr-un scan LAN. De aceea harta locației este desenată/confirmată manual; legăturile deduse automat sunt marcate distinct („inferat”) față de cele confirmate („confirmat”).
- Asocierea BSSID → IP al AP-ului este o estimare (ex. MAC-uri apropiate numeric) și trebuie confirmată manual.

---

## 3. Stack

| Componentă | Tehnologie |
|---|---|
| Backend / scanner | Go 1.22+ |
| Server web local | `net/http` din standard library (routing cu pattern-uri `GET /api/...` din Go 1.22) |
| Frontend inclus în binar | `embed` (`//go:embed web`) — un singur executabil, fără fișiere separate |
| Bază de date | SQLite prin `modernc.org/sqlite` (pure Go, fără CGO → cross-compile simplu) |
| Frontend | HTML + CSS + JavaScript vanilla, Canvas 2D |
| PDF | generat din HTML de raport (print CSS + `window.print()` din browser) |
| Scanare LAN | Linux: ARP cu `github.com/mdlayher/arp`; toate OS-urile: fallback ping sweep concurent + citirea tabelei ARP a sistemului |
| Concurență scan | goroutine-uri cu worker pool limitat (semafor pe canal) |
| Vendor MAC | fișier OUI local inclus cu `embed`, fără apeluri externe |
| Wi-Fi | Linux: `nmcli`; Windows: `netsh wlan show networks mode=bssid`; macOS: `system_profiler SPAirPortDataType -json` (apelate cu `os/exec`) |
| CLI | `flag` din standard library sau `github.com/spf13/cobra` |

Build:

```bash
go build -o netmap ./cmd/netmap
GOOS=windows GOARCH=amd64 go build -o netmap.exe ./cmd/netmap
GOOS=darwin  GOARCH=arm64 go build -o netmap-mac ./cmd/netmap
```

Comenzi CLI:

```bash
./netmap scan          # LAN + Wi-Fi, salvează în SQLite
./netmap scan --lan    # doar LAN
./netmap scan --wifi   # doar Wi-Fi
./netmap web           # pornește UI pe http://127.0.0.1:8765 și deschide browserul
./netmap export --format csv|json|html --project <id>
```

Serverul ascultă **doar pe 127.0.0.1**, niciodată pe 0.0.0.0.

Privilegii: ARP raw pe Linux cere root sau `sudo setcap cap_net_raw+ep ./netmap`. Fără privilegii, aplicația trece automat pe fallback (ping + tabela ARP) și afișează în UI ce metodă a folosit.

---

## 4. Structura proiectului

```
network-mapper/
├── cmd/netmap/
│   └── main.go              # CLI: scan | web | export
├── internal/
│   ├── config/config.go
│   ├── db/
│   │   ├── db.go            # conexiune SQLite + migrații
│   │   └── migrations/*.sql # incluse cu embed
│   ├── scanner/
│   │   ├── iface.go         # detectare interfață, subnet, gateway, DNS
│   │   ├── arp_linux.go     # ARP raw (build tag linux)
│   │   ├── arp_other.go     # fallback ping + tabela ARP (windows, darwin)
│   │   ├── ping.go          # ping sweep concurent
│   │   ├── hostname.go      # reverse DNS, mDNS
│   │   ├── ports.go         # TCP connect cu net.DialTimeout
│   │   ├── vendor.go        # lookup OUI (fișier embed)
│   │   └── classify.go      # ghicire tip dispozitiv
│   ├── wifi/
│   │   ├── wifi.go          # interfață comună
│   │   ├── nmcli_linux.go
│   │   ├── netsh_windows.go
│   │   └── airport_darwin.go
│   ├── api/
│   │   └── routes.go        # handler-e REST
│   └── export/
│       ├── csv.go
│       ├── json.go
│       └── html.go          # raport self-contained (html/template)
├── assets/oui.csv
├── web/                     # inclus în binar cu //go:embed
│   ├── index.html           # layout: sidebar + canvas + panou proprietăți
│   ├── css/app.css
│   └── js/
│       ├── app.js           # bootstrap, routing între tab-uri
│       ├── api.js           # fetch către backend
│       ├── editor/
│       │   ├── canvas.js    # render loop, zoom, pan, grilă
│       │   ├── scene.js     # modelul scenei (noduri, linii, pereți, texte)
│       │   ├── tools.js     # select, perete, cablu, text, cameră, radieră
│       │   ├── snap.js      # snap la grilă, unghiuri, porturi
│       │   ├── history.js   # undo / redo
│       │   └── icons.js     # toate icon-urile SVG
│       ├── devices.js       # tabel dispozitive
│       ├── wifi.js          # tabel Wi-Fi + grupare pe SSID
│       └── export.js
├── data/
│   └── network-map.db    # creat la prima rulare, lângă binar
├── go.mod
└── README.md
```

---

## 5. Model de date (SQLite)

```sql
CREATE TABLE projects (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,            -- ex. "Restaurant RP"
    address      TEXT,
    notes        TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE scans (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    type         TEXT NOT NULL,            -- lan | wifi | survey
    subnet       TEXT,
    interface    TEXT,
    gateway      TEXT,
    dns          TEXT,
    started_at   TEXT NOT NULL,
    finished_at  TEXT
);

-- Inventar persistent: cheia stabilă este MAC-ul
CREATE TABLE devices (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    mac          TEXT NOT NULL,
    last_ip      TEXT,
    hostname     TEXT,
    vendor       TEXT,
    detected_type TEXT,                    -- ghicit automat
    os_guess     TEXT,
    custom_name  TEXT,                     -- dat de utilizator
    device_type  TEXT,                     -- ales de utilizator (suprascrie detected_type)
    location     TEXT,                     -- Bar, Sală, Bucătărie...
    notes        TEXT,
    color        TEXT,
    first_seen   TEXT,
    last_seen    TEXT,
    UNIQUE (project_id, mac)
);

CREATE TABLE device_observations (
    id           INTEGER PRIMARY KEY,
    scan_id      INTEGER NOT NULL REFERENCES scans(id),
    device_id    INTEGER NOT NULL REFERENCES devices(id),
    ip           TEXT,
    open_ports   TEXT,                     -- JSON: [22,80,443]
    services     TEXT,                     -- JSON: {"22":"ssh"}
    latency_ms   REAL
);

CREATE TABLE wifi_networks (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    ssid         TEXT,
    bssid        TEXT NOT NULL,
    band         TEXT,                     -- 2.4 | 5 | 6
    channel      INTEGER,
    frequency    INTEGER,
    security     TEXT,
    vendor       TEXT,
    ap_device_id INTEGER REFERENCES devices(id),   -- asociere cu AP-ul din LAN
    ap_link_status TEXT DEFAULT 'none',    -- none | inferat | confirmat
    label        TEXT,                     -- ex. "AP Sală"
    location     TEXT,
    notes        TEXT,
    first_seen   TEXT,
    last_seen    TEXT,
    UNIQUE (project_id, bssid)
);

CREATE TABLE wifi_observations (
    id           INTEGER PRIMARY KEY,
    scan_id      INTEGER NOT NULL REFERENCES scans(id),
    wifi_id      INTEGER NOT NULL REFERENCES wifi_networks(id),
    signal_dbm   INTEGER,
    survey_point_id INTEGER REFERENCES survey_points(id)
);

-- Puncte de survey: utilizatorul se plimbă cu laptopul și marchează locul pe hartă
CREATE TABLE survey_points (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    label        TEXT,                     -- "Lângă bar"
    x            REAL NOT NULL,            -- coordonate în lumea hărții
    y            REAL NOT NULL,
    created_at   TEXT NOT NULL
);

-- Harta desenată (o hartă per etaj/zonă)
CREATE TABLE maps (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    name         TEXT NOT NULL,            -- "Parter", "Terasă"
    scene_json   TEXT NOT NULL,            -- întreaga scenă (vezi secțiunea 7.9)
    background   TEXT,                     -- imagine plan (data URI), opțional
    version      INTEGER NOT NULL DEFAULT 1,
    updated_at   TEXT NOT NULL
);
```

Scena editorului se salvează ca JSON într-un singur câmp (`maps.scene_json`) — simplu de versionat, exportat și reîncărcat. Legătura cu inventarul se face prin `device_id` în fiecare nod al scenei.

---

## 6. Scanner

### 6.1 LAN

1. Detectează interfața activă, IP-ul local, masca, gateway-ul și DNS-ul.
2. Calculează subnetul (ex. `192.168.1.0/24`). Dacă e mai mare de /22, cere confirmare.
3. ARP sweep (Linux, `mdlayher/arp`). Fallback: ping sweep concurent (goroutine-uri, max 128 simultan) + citirea tabelei ARP (`ip neigh` / `arp -a`).
4. Pentru fiecare host activ: MAC, vendor (OUI local), hostname (reverse DNS, mDNS, NetBIOS unde e disponibil).
5. TCP connect pe o listă scurtă de porturi, timeout 300 ms, concurență limitată (ex. 64):
   `21, 22, 23, 53, 80, 139, 443, 445, 515, 631, 3389, 5900, 8080, 8443, 9100, 554, 1883`
6. Clasificare automată (`classify.go`), ex.:
   - gateway → `router`
   - port 9100 / 515 / 631 → `imprimanta`
   - port 554 → `camera`
   - vendor Ubiquiti / TP-Link / Mikrotik + port 80/443 fără alte servicii → `ap` sau `router`
   - port 3389 → `pc` (Windows)
   - fără porturi deschise, vendor Apple/Samsung/Xiaomi → `telefon`
7. Upsert în `devices` după MAC; nu suprascrie niciodată `custom_name`, `device_type`, `location`, `notes`.
8. La rescan: dispozitivele care nu mai răspund rămân în inventar, marcate „offline” (după `last_seen`).

### 6.2 Wi-Fi

- Linux: `nmcli -t -f SSID,BSSID,CHAN,FREQ,SIGNAL,SECURITY dev wifi list --rescan yes`
- Windows: `netsh wlan show networks mode=bssid` (parsare text)
- macOS: `system_profiler SPAirPortDataType -json`
- Conversie semnal procentual → dBm aproximativ unde sistemul raportează procent.
- Bandă derivată din frecvență / canal (1–14 → 2.4 GHz, 36–177 → 5 GHz, 6 GHz după frecvență).
- Upsert în `wifi_networks` după BSSID.

### 6.3 Detectare AP-uri / amplificatoare non-mesh

Scenariu tipic: mai multe routere folosite ca amplificatoare, toate emit `RP` și `RP_5G`, fără mesh.

- Grupare pe SSID: afișează „SSID `RP` — 3 BSSID-uri → probabil 3 echipamente”.
- Pereche 2.4/5 GHz: BSSID-uri care diferă doar pe ultimul octet (sau ultimii 4 biți) sunt propuse ca „același echipament, radio diferit”.
- Asociere BSSID ↔ dispozitiv LAN: se compară BSSID-ul cu MAC-urile din LAN; dacă diferența numerică e mică (±8 pe ultimii octeți) și OUI identic → propunere cu status `inferat`.
- Utilizatorul confirmă sau respinge propunerea (status `confirmat`).
- Avertismente utile în raport: canale suprapuse pe 2.4 GHz (altele decât 1/6/11), două AP-uri pe același canal cu semnal puternic în același punct, AP-uri care rulează DHCP propriu (gateway diferit detectat).

### 6.4 Mod Survey

1. Utilizatorul deschide harta și dă click pe „Punct survey”.
2. Pune un marker pe hartă acolo unde stă fizic cu laptopul.
3. Aplicația rulează un scan Wi-Fi și salvează semnalul fiecărui BSSID la acel punct.
4. Se repetă în mai multe locuri.
5. Rezultat: pe hartă, lângă fiecare punct, cel mai puternic BSSID + dBm; opțional un heatmap simplu per SSID (interpolare IDW pe grilă, randat semi-transparent sub icon-uri).
6. Pe baza punctelor cu semnal maxim, aplicația poate sugera poziția aproximativă a fiecărui AP.

---

## 7. Editorul de hartă (canvas) — partea principală

### 7.1 Layout ecran

```
┌──────────────────────────────────────────────────────────────────────┐
│  Proiect: Restaurant RP ▾  │ Hartă: Parter ▾ │ Scan │ Salvează │ Export ▾ │
├───────────┬──────────────────────────────────────────┬───────────────┤
│ UNELTE    │                                          │ PROPRIETĂȚI   │
│ ↖ Select  │                                          │               │
│ ▭ Cameră  │             CANVAS (grilă)               │ Nume          │
│ ╱ Perete  │                                          │ Tip           │
│ ─ Cablu   │                                          │ Dispozitiv ▾  │
│ ⌒ Wi-Fi   │                                          │ (din inventar)│
│ T Text    │                                          │ Locație       │
│ ⌫ Șterge  │                                          │ Observații    │
├───────────┤                                          │ Culoare       │
│ ICON-URI  │                                          │               │
│ [căutare] │                                          │ IP / MAC      │
│ Rețea     │                                          │ Porturi       │
│ Stații    │                                          │ SSID/BSSID    │
│ POS       │                                          │               │
│ Periferice│                                          │               │
│ Mobil     │                                          │               │
├───────────┴──────────────────────────────────────────┴───────────────┤
│ Zoom 100% │ Grilă 20px ☑ │ Snap ☑ │ x: 340 y: 120 │ Undo Redo       │
└──────────────────────────────────────────────────────────────────────┘
```

Tab-uri principale: **Hartă** · **Dispozitive** · **Wi-Fi** · **Survey** · **Raport**.

### 7.2 Paleta de icon-uri

Icon-uri SVG proprii, stil flat, monocrome cu culoare de accent configurabilă, toate la 48×48 unități în coordonate de lume. Grupate pe categorii, cu căutare după nume:

| Categorie | Icon-uri |
|---|---|
| Rețea | Internet / ISP, Modem, Router, Router folosit ca AP / amplificator, Access Point (tavan), Switch, Patch panel, Rack, Firewall |
| Servere / stații | Server, NAS, PC, Laptop, Monitor, Terminal (thin client / stație de lucru) |
| POS / comercial | Casă de marcat, Terminal POS / card, Imprimantă bonuri, Imprimantă bucătărie, Tabletă ospătar, Kiosk self-order, Scanner coduri de bare, Cântar |
| Periferice | Imprimantă, Cameră IP, DVR/NVR, Telefon IP, Boxă / sistem audio, TV / Smart TV |
| Mobil | Telefon, Tabletă |
| Infrastructură | UPS, Priză rețea (port de perete), Priză electrică, Tablou electric |
| Generic | Dispozitiv necunoscut, Notă / etichetă |

Interacțiune:
- **Drag & drop** din paletă pe canvas (icon-ul urmărește cursorul, cu previzualizare semi-transparentă și snap la grilă).
- Alternativ: click pe icon în paletă, apoi click pe canvas pentru plasare.
- Dispozitivele din inventar (tab Dispozitive) pot fi trase direct pe hartă → nodul primește automat `device_id`, icon-ul corespunzător tipului și eticheta `custom_name` sau IP.
- Dispozitivele din inventar încă neplasate pe hartă apar într-o listă „Neplasate” în sidebar.

### 7.3 Nod (echipament) pe canvas

- Icon + etichetă sub icon (nume personalizat, apoi IP pe rândul doi, opțional).
- Indicator de stare: punct verde (online la ultimul scan) / gri (offline) / fără punct (nod nelegat de inventar).
- Handle-uri: mutare, rotire în pași de 90°, redimensionare proporțională (scala 0.5×–3×).
- **Porturi de conectare** (anchor points) pe cele 4 laturi ale icon-ului, vizibile la hover când unealta Cablu este activă.
- Dublu-click → deschide panoul de proprietăți cu focus pe nume.

### 7.4 Unealta Cablu (linii drepte)

Cerința principală: linii **drepte**, curate, astfel încât harta să arate exact ca locația.

- Click pe un port al nodului A → click pe portul nodului B = cablu legat de noduri (se mută odată cu ele).
- Click pe spațiu gol = punct intermediar (colț). Dublu-click sau `Enter` = termină linia. `Esc` = anulează.
- Moduri de trasare (comutabile din bara de proprietăți sau cu tasta `M`):
  - **Drept**: segment direct între puncte.
  - **Ortogonal** (implicit): doar segmente orizontale și verticale; colțurile se calculează automat în L sau Z.
- Ținând apăsat `Shift`: constrângere la 0° / 45° / 90°.
- Snap: la grilă, la porturile nodurilor, la capetele și mijlocul altor segmente, la pereți (aliniere).
- Ghidaje de aliniere (linii subțiri punctate) când punctul curent e aliniat cu alt nod sau punct.
- Punctele intermediare pot fi mutate ulterior (drag); click pe segment + `Alt` = adaugă punct; `Del` pe punct = elimină punctul.
- Tipuri de cablu (culoare și stil diferite):
  - Ethernet — linie continuă, albastru
  - Fibră — linie continuă, portocaliu
  - Wi-Fi / legătură wireless — linie punctată, verde
  - Alimentare / PoE — linie întreruptă, roșu
  - Necunoscut / inferat — linie punctată, gri
- Proprietăți cablu: tip, etichetă (ex. „Cat6, 15 m”), port sursă / port destinație (text liber: „SW port 4”), status `confirmat` / `inferat`, observații.
- Opțional: afișare lungime calculată a cablului pe baza scării hărții (vezi 7.6).

### 7.5 Pereți și camere (planul locației)

- **Unealta Perete**: linii groase (grosime configurabilă, implicit 8 unități), cu aceleași reguli de snap și constrângere unghiulară ca la cabluri. Pereții se unesc curat la colțuri.
- **Unealta Cameră**: drag pentru dreptunghi; umplere semi-transparentă și nume centrat („Sală”, „Bar”, „Bucătărie”, „Terasă”, „Depozit”). Nodurile plasate în interiorul unei camere primesc automat `location` = numele camerei (suprascriibil).
- Uși / ferestre: simboluri simple plasabile pe pereți (opțional, faza 4).
- **Imagine de fundal**: încărcare plan (PNG/JPG/PDF prima pagină), opacitate reglabilă, blocare poziție — utilizatorul poate trasa pereții peste planul real.

### 7.6 Grilă, scară, navigare

- Grilă cu pas configurabil (10 / 20 / 40 px), cu linii majore la fiecare 5 pași.
- Scară opțională: utilizatorul trasează o linie pe o distanță cunoscută și introduce metrii → 1 unitate = X m. Se afișează o riglă în colț.
- Zoom cu rotița (centrat pe cursor), 10%–400%. Pan cu `Space` + drag sau click-mijloc.
- „Încadrează tot” (`F`) și minimap opțional.

### 7.7 Selecție și editare

- Click = selectează; `Shift`+click = adaugă la selecție; drag pe gol = selecție dreptunghiulară.
- Mutare cu mouse-ul sau cu săgețile (1 pas grilă; `Shift` = 5 pași).
- Aliniere / distribuire pentru selecție multiplă: stânga, centru, dreapta, sus, mijloc, jos, distribuie orizontal/vertical.
- Copiere / lipire / duplicare (`Ctrl+C`, `Ctrl+V`, `Ctrl+D`).
- Straturi (layers) cu vizibilitate și blocare: Fundal, Pereți & camere, Cabluri, Echipamente, Wi-Fi & survey, Etichete.
- Ștergerea unui element de pe hartă **nu** șterge dispozitivul din inventar.

### 7.8 Istoric și salvare

- Undo / redo (`Ctrl+Z`, `Ctrl+Y`), minim 100 de pași, bazat pe comenzi (command pattern).
- Salvare manuală (`Ctrl+S`) + autosalvare la 30 de secunde dacă există modificări.
- Indicator „Nesalvat” în bara de sus; avertisment la închiderea tab-ului.
- Versiune incrementată la fiecare salvare; optimistic locking pe câmpul `version`.

### 7.9 Format `scene_json`

```json
{
  "schema": 1,
  "grid": { "size": 20, "snap": true },
  "scale": { "unitsPerMeter": 40 },
  "view": { "zoom": 1, "panX": 0, "panY": 0 },
  "layers": [
    { "id": "walls", "visible": true, "locked": false }
  ],
  "rooms": [
    { "id": "r1", "name": "Sală", "x": 0, "y": 0, "w": 600, "h": 400, "color": "#e8f0fe" }
  ],
  "walls": [
    { "id": "w1", "points": [[0,0],[600,0],[600,400]], "thickness": 8 }
  ],
  "nodes": [
    {
      "id": "n1",
      "icon": "router",
      "x": 120, "y": 80,
      "rotation": 0, "scale": 1,
      "label": "Router principal",
      "device_id": 12,
      "wifi_bssids": ["AA:AA:AA:01:01:01", "AA:AA:AA:01:01:02"],
      "color": "#1a73e8"
    },
    {
      "id": "n2",
      "icon": "casa_de_marcat",
      "x": 420, "y": 300,
      "label": "Casă bar",
      "device_id": 18
    }
  ],
  "links": [
    {
      "id": "l1",
      "from": { "node": "n1", "port": "right" },
      "to":   { "node": "n2", "port": "top" },
      "points": [[168,104],[440,104]],
      "mode": "orthogonal",
      "type": "ethernet",
      "label": "Cat6",
      "status": "confirmat"
    }
  ],
  "texts": [
    { "id": "t1", "x": 300, "y": 20, "text": "Intrare", "size": 14 }
  ],
  "surveyPoints": [
    { "id": "s1", "survey_point_id": 3, "x": 250, "y": 200 }
  ]
}
```

### 7.10 Implementare canvas

- Un singur `<canvas>` cu `devicePixelRatio` aplicat pentru claritate pe ecrane HiDPI.
- Coordonate de lume separate de coordonatele ecranului (transformare zoom/pan într-un singur loc).
- Randare doar la nevoie (`requestAnimationFrame` declanșat de modificări, nu buclă continuă).
- Icon-urile SVG sunt pre-randate în `ImageBitmap` / `Image` per culoare și dimensiune și păstrate în cache.
- Hit-testing: noduri (bounding box), segmente (distanță punct–segment < 6 px pe ecran), porturi (rază 8 px).
- Ordinea de desenare: fundal → grilă → camere → pereți → heatmap → cabluri → noduri → etichete → selecție și ghidaje.
- Performanță țintă: 500 de noduri + 1000 de segmente fără lag la pan/zoom.

### 7.11 Scurtături tastatură

| Tastă | Acțiune |
|---|---|
| `V` | Select |
| `W` | Perete |
| `R` | Cameră |
| `C` | Cablu |
| `T` | Text |
| `M` | Comută drept / ortogonal |
| `Shift` | Constrângere 0/45/90° |
| `G` | Grilă on/off |
| `S` | Snap on/off |
| `F` | Încadrează tot |
| `Del` | Șterge selecția |
| `Esc` | Anulează operația curentă |
| `Ctrl+Z / Ctrl+Y` | Undo / Redo |
| `Ctrl+S` | Salvează |

---

## 8. Tab Dispozitive

- Tabel sortabil și filtrabil: status, nume, tip, IP, MAC, vendor, hostname, porturi, locație, pe hartă (da/nu), ultima vedere.
- Editare inline: nume, tip, locație, observații, culoare.
- Buton „Arată pe hartă” (centrează canvas-ul pe nod).
- Evidențiere dispozitive noi față de scanul anterior și dispozitive dispărute.

## 9. Tab Wi-Fi

- Vedere grupată pe SSID:

```
RP                 3 BSSID-uri
├── AA:AA:AA:01:01:01   ch 1    2.4 GHz   -42 dBm   → Router principal (confirmat)
├── AA:AA:AA:02:02:02   ch 6    2.4 GHz   -57 dBm   → 192.168.1.10 (inferat) [Confirmă] [Respinge]
└── AA:AA:AA:03:03:03   ch 11   2.4 GHz   -71 dBm   → neasociat
RP_5G              3 BSSID-uri
└── ...
```

- Editare: etichetă („AP Sală”), locație, observații, asociere manuală cu un dispozitiv din LAN.
- Grafic simplu de canale (bare pe canal, 2.4 și 5 GHz separat) pentru a vedea suprapunerile.

## 10. API REST (local)

```
GET    /api/projects
POST   /api/projects
GET    /api/projects/{id}

POST   /api/projects/{id}/scan            body: {"lan": true, "wifi": true}
GET    /api/scans/{id}                    status + progres (polling)

GET    /api/projects/{id}/devices
PATCH  /api/devices/{id}                  nume, tip, locație, observații, culoare

GET    /api/projects/{id}/wifi
PATCH  /api/wifi/{id}                     etichetă, locație, ap_device_id, ap_link_status

POST   /api/projects/{id}/survey-points   body: {"label","x","y"} → rulează scan Wi-Fi
GET    /api/projects/{id}/survey-points

GET    /api/projects/{id}/maps
POST   /api/projects/{id}/maps
GET    /api/maps/{id}
PUT    /api/maps/{id}                     body: {"scene_json", "version"}

GET    /api/projects/{id}/export?format=csv|json|html
```

Ștergerile (proiect, hartă, dispozitiv) au endpoint-uri separate și cer confirmare în UI.

## 11. Export și raport

| Format | Conținut |
|---|---|
| **PNG** | harta la rezoluție aleasă (1×, 2×, 4×), fundal alb, cu legendă |
| **SVG** | harta vectorială (re-randare a scenei în SVG, nu captură canvas) |
| **CSV** | `name,type,ip,mac,vendor,hostname,os,ports,location,notes,last_seen` |
| **JSON** | backup complet proiect: dispozitive, Wi-Fi, survey, hărți |
| **HTML** | fișier self-contained (`network-map-AAAA-LL-ZZ.html`): hartă SVG inline + tabele + info, fără dependențe |
| **PDF** | raport imprimabil |

Structură raport PDF:
1. Copertă: nume locație, adresă, dată, autor.
2. Rezumat: nr. dispozitive, nr. AP-uri, SSID-uri detectate, gateway, subnet, DNS.
3. Harta locației (pe o pagină, landscape) + legendă icon-uri și tipuri de cablu.
4. Infrastructură wireless: fiecare AP cu SSID-uri, BSSID-uri, canale, locație.
5. Tabel dispozitive grupat pe locație.
6. Observații și recomandări detectate automat (canale suprapuse, AP-uri pe același canal, dispozitive necunoscute).
7. Anexă: rezultate survey pe puncte.

Importul unui JSON de backup trebuie să fie posibil (restaurare proiect).

## 12. Faze de dezvoltare

**Faza 1 — Fundație**
- Structura proiectului, SQLite + migrații, CLI, server local, pagină goală cu tab-uri.
- Criteriu: `go build` produce un singur executabil; `./netmap web` pornește și afișează UI-ul.

**Faza 2 — Editor canvas (prioritar)**
- Grilă, zoom, pan, paleta de icon-uri, drag & drop, selecție, mutare.
- Unealta Cablu cu moduri drept/ortogonal, snap, porturi, `Shift` pentru unghiuri.
- Pereți și camere, texte, undo/redo, salvare/încărcare `scene_json`.
- Criteriu: se poate desena complet planul unui restaurant cu 10 echipamente și cabluri drepte, salva, reîncărca identic.

**Faza 3 — Scanner LAN + inventar**
- Scan ARP/ping, vendor, hostname, porturi, clasificare, upsert după MAC.
- Tab Dispozitive, trage dispozitiv din inventar pe hartă, stare online/offline pe noduri.

**Faza 4 — Wi-Fi**
- Scan Wi-Fi pe Linux (apoi Windows, macOS, cu fișiere separate pe build tags), grupare pe SSID, asociere BSSID ↔ AP inferată + confirmare.
- Mod Survey cu puncte pe hartă; heatmap opțional.

**Faza 5 — Export**
- PNG, SVG, CSV, JSON (export + import), HTML self-contained, raport PDF.

**Faza 6 — Finisare**
- Uși/ferestre, minimap, aliniere/distribuire, scară în metri și lungime cabluri, teme de culoare.

## 13. Criterii de acceptare generale

- Un singur binar per sistem de operare, fără runtime instalat (fără Python/Node).
- Rulează complet offline; nicio cerere către internet (inclusiv fonturi și icon-uri).
- Numele, locațiile și observațiile date manual supraviețuiesc oricărui rescan.
- O hartă salvată și redeschisă arată identic (poziții, linii, colțuri, culori).
- Liniile de cablu rămân atașate de noduri când nodurile sunt mutate, păstrându-și forma ortogonală.
- Interfața este utilizabilă la 1366×768 și pe ecrane mari.
- Nicio operație de ștergere fără confirmare în UI.
