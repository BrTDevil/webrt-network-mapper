# webrt-network-mapper

> Go 1.22+ · `net/http` · SQLite (`modernc.org/sqlite`, fără CGO) · frontend HTML/CSS/JS vanilla inclus în binar · un singur executabil, 100% offline

Aplicație locală pentru **inventarul unei rețele** (LAN + Wi-Fi) și **harta fizică a locației** (restaurant, birou, magazin). Scanezi rețeaua, desenezi planul localului cu echipamentele și cablurile dintre ele, salvezi totul ca **instantaneu** într-un folder, iar peste câteva luni îl compari cu starea de atunci.

- Descoperă dispozitivele din rețea: IP, MAC, producător, hostname, porturi deschise și tipul ghicit (router, imprimantă, cameră, PC, telefon…).
- Inventariază rețelele Wi-Fi: SSID, BSSID, canal, bandă, semnal. Grupează BSSID-urile pe SSID („RP — 3 BSSID-uri → probabil 3 echipamente”) și deduce ce router emite ce rețea.
- Are un editor de hartă cu grilă: pereți, camere, uși/ferestre, 40 de icon-uri distincte (router, router folosit ca amplificator, cameră IP, imprimantă de bonuri, casă de marcat, terminal ospătar, PC cu bază de date de produse…) și cabluri drepte sau ortogonale care rămân legate de echipamente.
- Face survey Wi-Fi: te plimbi cu laptopul, marchezi punctele pe hartă și primești un heatmap al semnalului.
- Salvează **instantanee** ca fișiere `.netmap.json` într-un folder ales de tine. Le poți redenumi și compara („ce s-a schimbat în 3 luni”).
- Exportă raport PDF, hartă PNG/SVG, CSV pentru Excel, backup JSON și raport HTML self-contained.

Serverul ascultă **doar pe 127.0.0.1**. Aplicația nu face nicio cerere către internet. Icon-urile și baza de producători MAC (IEEE OUI) sunt incluse în binar, iar interfața folosește fonturile sistemului.

---

## Cuprins

1. [Instalare globală pe laptop](#1-instalare-globală-pe-laptop)
2. [Pornire rapidă](#2-pornire-rapidă)
3. [Cum se folosește](#3-cum-se-folosește)
4. [Instantanee și comparare în timp](#4-instantanee-și-comparare-în-timp)
5. [Linia de comandă (CLI)](#5-linia-de-comandă-cli)
6. [Unde stau datele](#6-unde-stau-datele)
7. [Scurtături tastatură](#7-scurtături-tastatură)
8. [Probleme frecvente](#8-probleme-frecvente)
9. [Arhitectură](#9-arhitectură)
10. [API REST local](#10-api-rest-local)
11. [Referință rapidă fișiere](#11-referință-rapidă-fișiere)

---

## 1. Instalare globală pe laptop

### Cerințe

| Ce | De ce | Verificare |
|---|---|---|
| Go 1.22+ | doar pentru compilare | `go version` |
| `make` | comenzile de instalare | `make --version` |
| NetworkManager (`nmcli`) | scanarea Wi-Fi pe Linux | `nmcli --version` |
| BlueZ (`bluetoothctl`) | scanarea Bluetooth (opțional) | `bluetoothctl --version` |
| `setcap` (pachetul `libcap2-bin`) | scan ARP complet fără root (opțional) | `which setcap` |

Pe Ubuntu/Debian, dacă lipsește ceva: `sudo apt install golang make network-manager libcap2-bin`.

### Varianta recomandată: `/usr/local/bin` (cu sudo)

```bash
cd /var/www/html/webrt.eu/angajare/go/network-mapper
make install
```

Ce face `make install`:

1. Compilează binarul (`CGO_ENABLED=0`, cu versiunea înglobată).
2. Îl copiază în `/usr/local/bin/webrt-network-mapper` (cere parola de sudo).
3. Rulează `sudo setcap cap_net_raw+ep` pe binar, ca scanarea ARP să meargă **fără sudo** la fiecare pornire.
4. Adaugă „Network Mapper” în meniul de aplicații (fișier `.desktop` + icon în `~/.local/share`).

După instalare, comanda merge din orice folder:

```bash
webrt-network-mapper            # deschide interfața în browser
webrt-network-mapper info       # verifică instalarea (versiune, date, rețea, ARP raw)
```

`info` trebuie să afișeze `ARP raw: disponibil (cap_net_raw)`. Dacă apare „indisponibil”, vezi [Probleme frecvente](#8-probleme-frecvente).

### Varianta fără sudo: `~/.local/bin`

```bash
make install-user
```

Binarul ajunge în `~/.local/bin`. Dacă folderul nu e în `PATH`, adaugă în `~/.bashrc`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

În varianta asta, scanarea LAN folosește ping + tabela ARP a sistemului, fără ARP raw. Găsește aproape toate dispozitivele. Pot lipsi doar cele care nu răspund la ping (unele telefoane în sleep).

### Actualizare

Recompilezi și reinstalezi. Datele și instantaneele rămân neatinse:

```bash
cd /var/www/html/webrt.eu/angajare/go/network-mapper
git pull            # dacă e cazul
make install
```

`setcap` se reaplică automat, pentru că fiecare binar nou își pierde capabilitățile.

### Dezinstalare

```bash
make uninstall          # varianta din /usr/local/bin
make uninstall-user     # varianta din ~/.local/bin
```

Dezinstalarea **nu** șterge datele: baza de date rămâne în `~/.local/share/webrt-network-mapper/`, iar instantaneele în `~/Documente/webrt-network-mapper/`. Dacă vrei să le ștergi, o faci manual.

### Binare pentru alte calculatoare

```bash
make dist
```

Produce în `dist/` câte un binar pentru Linux (amd64/arm64), Windows (`.exe`) și macOS (Intel/Apple Silicon). Nu trebuie instalat nimic pe calculatorul țintă: copiezi fișierul și îl rulezi.

---

## 2. Pornire rapidă

```bash
webrt-network-mapper
```

1. Pornește serverul pe **http://127.0.0.1:8765** și deschide browserul automat.
2. La prima pornire creezi un proiect. Un proiect = o locație, de ex. „Restaurant RP”.
3. Apeși **Scan** (sus, dreapta), bifezi LAN și Wi-Fi și confirmi că rețeaua e a ta.
4. **Harta se generează automat din prima:** Internet → router (gateway) → dispozitivele pe coloane (Rețea, Stații & servere, POS, Periferice, Mobile & Wi-Fi, Neidentificate), fiecare legat de router. Apoi le muți unde sunt fizic, desenezi pereții/camerele și confirmi cablurile.
5. Oprești serverul cu `Ctrl+C` în terminal. Harta se salvează automat la 30 de secunde, iar `Ctrl+S` salvează imediat.

Dacă aplicația rulează deja și o pornești din nou, doar se deschide browserul pe instanța existentă.

Din meniul de aplicații („Network Mapper”) pornește fără terminal. Serverul se oprește la închiderea sesiunii sau cu `pkill webrt-network-mapper`.

Alte opțiuni:

```bash
webrt-network-mapper web --addr 127.0.0.1:9000   # alt port
webrt-network-mapper web --no-browser            # fără deschiderea browserului
```

---

## 3. Cum se folosește

Tab-uri: **Hartă · Dispozitive · Wi-Fi · Bluetooth · Survey · Istoric · Raport**.

### Locații și etaje — ce e fiecare

| În bara de sus | Ce este | Ce are separat |
|---|---|---|
| **Locație** (meniul `⋯` de lângă ea) | Un loc fizic cu rețeaua lui: acasă, un restaurant, un birou. În API și CLI se numește „proiect”. | **Tot**: inventarul de dispozitive, rețelele Wi-Fi, Bluetooth, scanările și etajele. |
| **Etaj / zonă** (meniul `⋯` de lângă el) | O hartă desenată **în interiorul** locației curente: Parter, Etaj 1, Terasă. | Doar desenul (pereți, camere, poziții). Inventarul e comun cu celelalte etaje ale locației. |

- **Alt loc = Locație nouă.** Pornește goală, fără nimic din celelalte locații.
- **Ștergerea unei locații** (`⋯` → „Șterge locația”) șterge tot ce ține de ea. După aceea, o locație nouă pornește de la zero, chiar dacă primește același ID.
- **Protecție la amestecare:** o locație își ține minte routerul sau routerele după **MAC**. Adresa IP nu ajunge, pentru că aproape toate routerele sunt `192.168.1.1`. Dacă pornești un scan în locația „Acasă”, dar laptopul e conectat la rețeaua altui loc, scanul **nu pornește**. Apare dialogul „Altă rețea decât a locației”, din care alegi:
  - **Deschide „X” și scanează**: rețeaua a mai fost scanată în locația X, iar scanul merge acolo;
  - **Locație nouă și scanează**: creezi locul nou și scanezi în el;
  - **Scanează totuși aici**: e același loc cu un al doilea router, iar routerul nou se adaugă la locație.

  O locație fără niciun scan LAN nu are încă router cunoscut, deci primul scan trece fără întrebări. Scanul cu „Altă rețea (SSID + parolă)” nu e verificat, pentru că acolo ai ales explicit rețeaua.

### Hartă — desenezi locația

| Unealtă | Tastă | Ce face |
|---|---|---|
| Select | `V` | selectare, mutare, selecție dreptunghiulară, `Shift`+click = adaugă la selecție |
| Cameră | `R` | tragi un dreptunghi („Sală”, „Bar”, „Bucătărie”, „Terasă”) |
| Perete | `W` | click pentru fiecare colț, dublu-click / `Enter` = termină |
| Cablu | `C` | click pe portul echipamentului A → click-uri pentru colțuri → click pe portul lui B |
| Text | `T` | etichete libere („Intrare”, „Depozit”) |
| Ușă / Fereastră | `D` | click lângă un perete: se aliniază automat cu el |
| Survey | `P` | punct de măsurare Wi-Fi (vezi mai jos) |
| Scară | — | trasezi o distanță cunoscută și introduci metrii → riglă și lungimi reale |
| Șterge | `E` | click pe element = îl scoate de pe hartă |

**Icon-urile** sunt grupate pe categorii și au căutare („ospătar”, „cameră”, „imprimantă”). Le tragi pe hartă sau dai click pe icon, apoi click pe hartă.

| Categorie | Icon-uri |
|---|---|
| Rețea | Internet/ISP, Modem, Router, **Router ca AP / amplificator**, Access Point (tavan), Switch, Patch panel, Rack, Firewall |
| Servere / stații | Server, NAS, PC, Laptop, Monitor, Terminal, **Computer bază de date produse** |
| POS / comercial | **Casă de marcat**, Terminal POS / card, **Imprimantă bonuri**, Imprimantă bucătărie, **Tabletă ospătar**, **Terminal ospătar (handheld)**, Kiosk, Scanner coduri, Cântar |
| Periferice | Imprimantă, **Cameră IP**, Cameră dome, DVR/NVR, Telefon IP, Boxă, TV |
| Mobil / Infrastructură / Generic | Telefon, Tabletă, UPS, Priză rețea, Priză electrică, Tablou electric, Necunoscut, Notă |

Fiecare echipament are o culoare proprie, mărime (0.5×–3×) și rotire în pași de 90°.

**Cablurile** sunt de două feluri:
- **Ortogonale** (implicit): doar orizontal/vertical, colțurile se calculează singure.
- **Drepte**: `M` comută între ele.

`Shift` constrânge la 0/45/90°. Cablurile rămân legate de echipamente când le muți. Tipuri: Ethernet (albastru), Fibră (portocaliu), Wi-Fi (verde punctat), PoE (roșu întrerupt), Inferat (gri punctat). Fiecare cablu are etichetă („Cat6, 15 m”), port sursă/destinație („SW port 4”) și status confirmat/inferat. Colțurile se mută cu mouse-ul; `Alt`+click pe cablu adaugă un colț, iar `Del` pe un colț selectat îl elimină.

**Aranjare automată:** după primul scan (sau la deschiderea primei hărți a proiectului, goală, cu inventar), harta se construiește singură. **O hartă nouă (etaj / zonă) pornește goală:** fiecare dispozitiv stă pe un singur etaj, așa că aranjarea automată (la scan sau din buton) nu mai pune pe harta curentă dispozitivele deja plasate pe alte hărți — doar pe cele neplasate nicăieri. Le muți pe etajul nou trăgându-le din panoul „Neplasate”, sau folosești **Duplică harta** dacă vrei o copie. La scanările următoare, dispozitivele **noi** se adaugă automat și se leagă de routerul existent. Butonul **⚡ Aranjează automat** (panoul „Neplasate”) face același lucru oricând. Legăturile sunt **deduse**, pentru că un scan nu poate vedea cablurile fizice: telefoanele, tabletele, laptopurile și terminalele de ospătar primesc o legătură **Wi-Fi** (verde punctat), restul un cablu **inferat** (gri punctat). Când verifici un cablu, îi schimbi tipul în Ethernet și statusul în „Confirmat”. Dacă ai și un switch sau un router-amplificator, muți capătul cablului de pe router pe el. `Ctrl+Z` anulează aranjarea.

**Legătura cu inventarul:** un echipament tras din lista „Neplasate” e legat de dispozitivul din rețea, identificat după MAC. Pe hartă apare un punct **verde** dacă a răspuns la ultimul scan și **gri** dacă nu. Un echipament pus într-o cameră primește automat locația = numele camerei. Ștergerea de pe hartă nu șterge dispozitivul din inventar.

**Plan de fundal:** când nu e nimic selectat, panoul din dreapta → „Încarcă plan…” (PNG/JPG). Reglezi opacitatea și trasezi pereții peste el.

**Straturi:** Fundal, Pereți & camere, Cabluri, Echipamente, Wi-Fi & survey, Etichete. Fiecare poate fi ascuns sau blocat.

**Mai multe hărți** per proiect (Parter, Terasă, Etaj 1): meniul `⋯` de lângă selectorul „Hartă”.

### Dispozitive

Tabelul se poate sorta și filtra: toate / online / offline / noi / neplasate / neidentificate. Editezi direct în tabel numele, tipul, locația, observațiile și culoarea. **Ce scrii tu nu se pierde niciodată la un scan nou**, pentru că dispozitivele sunt identificate după MAC, iar IP-ul se poate schimba prin DHCP. Dispozitivele apărute la ultimul scan sunt marcate **NOU**. „+ Dispozitiv manual” e pentru echipamentele care nu apar la scan, cum ar fi switch-urile nemanaged sau prizele de rețea.

### Wi-Fi

Rețelele sunt grupate pe SSID, cu numărul estimat de echipamente. Aplicația propune ce router din LAN emite fiecare BSSID (status **inferat**), iar tu apeși **Confirmă** sau **Respinge**. Pentru fiecare BSSID poți pune etichetă, locație și observații. Graficul de canale pe 2.4 și 5 GHz și avertismentele (canal suprapus, 2 AP-uri puternice pe același canal) arată unde e interferență.

### Bluetooth

**Scan → bifează „Bluetooth”**, sau tab-ul **Bluetooth → „Scan Bluetooth”**. Aplicația **doar ascultă ~12 secunde** anunțurile pe care dispozitivele din jur le emit singure: nume, tip, producător, semnal. Nu se conectează, nu împerechează nimic și nu încearcă PIN-uri. Merge pe Linux, prin BlueZ (`bluetoothctl`).

- Tipul e ghicit din nume: imprimantă de bonuri, terminal POS, scanner de coduri, căști, boxă, mouse/tastatură, ceas/brățară, telefon, laptop.
- Un dispozitiv Bluetooth cu **același nume** ca unul din LAN (de ex. `DESKTOP-QAAOIPI`) e legat automat de el (status *inferat*, îl confirmi sau îl respingi).
- Dispozitivele **fără nume** au adrese aleatoare, de obicei telefoane sau ceasuri ale trecătorilor. Sunt ascunse implicit (bifa „Doar cu nume”).
- **„Pune pe hartă”** adaugă dispozitivul cu punct mov. Legătura o desenezi cu unealta Cablu, tipul **Bluetooth** (mov punctat).

### Scan pe o rețea după nume și parolă

Scanul LAN vede doar rețeaua la care e conectat laptopul. Pentru altă rețea a ta (de ex. `RP_Bucătărie`):

1. **Scan → „Rețeaua pentru scanul LAN” → Altă rețea: …** (sau „scriu numele”).
2. Introduci **parola rețelei**. O lași goală dacă laptopul s-a mai conectat la ea și o are salvată.
3. Laptopul se deconectează temporar, se conectează la rețeaua aleasă, scanează și **revine singur** la rețeaua de dinainte, chiar și dacă scanul eșuează.

Parola e **a ta, dată de tine**. Aplicația nu ghicește nimic și nu face brute force. Parola e trimisă lui NetworkManager prin stdin, nu apare în lista de procese, **nu se salvează** în baza de date, în loguri sau în instantanee, iar profilul Wi-Fi temporar e șters după scan. Funcția e disponibilă doar pe Linux (NetworkManager).

**„Wi-Fi: doar rețelele mele (după nume)”:** filtrul (de ex. `RP`) păstrează la scanul Wi-Fi doar rețelele al căror nume conține termenul (`RP`, `RP_5G`, `RP-Terasa`) și le ignoră pe ale vecinilor. Filtrul se reține per proiect.

### Survey

1. Mergi cu laptopul în locul unde vrei să măsori (lângă bar, în sală, pe terasă).
2. **Survey → „+ Punct survey pe hartă”** și dai click pe hartă exact unde stai.
3. Aplicația rulează un scan Wi-Fi și salvează semnalul fiecărei rețele în acel punct.
4. Repeți în cât mai multe locuri. Alegi un SSID ca să vezi **heatmap-ul**, iar bifa „poziția estimată a AP-urilor” le marchează pe hartă.

### Raport și export

| Format | Conținut |
|---|---|
| **PDF** | „Tipărește / salvează PDF” → dialogul de print al browserului → *Salvează ca PDF*. Conține copertă, rezumat, harta (pagină landscape, cu legendă), infrastructura wireless, dispozitivele pe locații, recomandări și anexa survey |
| **PNG** 1×/2×/4× | harta pe fundal alb, cu legendă |
| **SVG** | harta vectorială |
| **CSV** | `name,type,ip,mac,vendor,hostname,os,ports,location,notes,last_seen` (se deschide corect în Excel, cu diacritice) |
| **JSON** | backup complet al proiectului, importabil din „Importă backup…” |
| **HTML** | raport într-un singur fișier, fără dependențe |

---

## 4. Instantanee și comparare în timp

Un instantaneu este **un fișier** cu tot proiectul la un moment dat: dispozitive, Wi-Fi, hărți și survey.

**Salvare:** butonul **Instantaneu** (sus) sau **Istoric → „Salvează instantaneu acum”**. Numele sugerat e data + proiectul, de ex. `2026-09-23 Restaurant RP`. Opțional, pui și un subfolder și observații („înainte de schimbarea routerului din bar”).

**Unde se salvează:** `~/Documente/webrt-network-mapper/<subfolder>/<nume>.netmap.json`, sau `~/Documents/…` dacă sistemul e în engleză. Folderul se schimbă din **Istoric → „Schimbă folderul”** sau din linia de comandă cu `webrt-network-mapper snapshot dir <cale>`.

**Redenumire:** din aplicație (**Redenumește**) sau direct din managerul de fișiere. Numele afișat e numele fișierului. Poți muta fișierele în subfoldere, iar lista se actualizează cu **↻**.

**Comparare:** în **Istoric**, alegi *Înainte (A)* și *După (B)*. Fiecare poate fi un instantaneu sau **starea curentă** a unui proiect. Primești:

- dispozitive **noi**, **dispărute** și **modificate**, cu fiecare schimbare: IP, nume, tip, locație, porturi, online/offline;
- rețele Wi-Fi noi, dispărute și modificate (SSID, canal, securitate);
- pe hărți: echipamente adăugate, scoase și mutate, numărul de cabluri;
- **cele 2 hărți una lângă alta**.

Butonul **„Compară cu acum”** de pe fiecare rând face direct comparația instantaneu → azi.

Din rândul unui instantaneu mai ai: **Raport** (raportul complet din acel moment), **⤓** (descarcă fișierul) și **Restaurează** (creează un proiect nou din instantaneu; proiectul curent nu e modificat).

Flux recomandat: la vizita de azi dai **Scan**, aranjezi harta și salvezi instantaneul `2026-09 Restaurant RP`. Peste 3 luni dai din nou **Scan**, apoi **Istoric → „Compară cu acum”** pe instantaneul din septembrie.

---

## 5. Linia de comandă (CLI)

```text
webrt-network-mapper web      [--addr 127.0.0.1:8765] [--no-browser]
webrt-network-mapper scan     [--lan] [--wifi] [--bt] [--project <id|nume>] [--ssid-filter "RP"] [--connect <SSID>]
                              [--iface wlan0] [--subnet 192.168.1.0/24] [--yes] [--other-network]
webrt-network-mapper export   --format csv|json|html --project <id|nume> [--out fișier] [--author "Nume"]
webrt-network-mapper import   <backup.netmap.json> [--name "Nume nou"]
webrt-network-mapper snapshot save --project <id|nume> [--name "..."] [--note "..."] [--folder "..."]
webrt-network-mapper snapshot list
webrt-network-mapper snapshot dir [<cale nouă>]
webrt-network-mapper projects
webrt-network-mapper info
webrt-network-mapper version
```

Fără argumente, echivalent cu `web`.

Exemple:

```bash
# scan din terminal (fără interfață); creează proiectul dacă nu există
webrt-network-mapper scan --project "Restaurant RP"

# doar Wi-Fi, doar rețelele proprii
webrt-network-mapper scan --wifi --ssid-filter "RP" --project "Restaurant RP"

# LAN + Bluetooth
webrt-network-mapper scan --lan --bt --project "Restaurant RP"

# LAN-ul altei rețele (parola se cere ascuns, nu se salvează)
webrt-network-mapper scan --connect "RP_Bucatarie" --project "Restaurant RP"

# al doilea router al aceleiași locații: fără --other-network, scanul se oprește
# (routerul nu a mai fost văzut în locație) și îți spune în ce locație e rețeaua
webrt-network-mapper scan --project "Restaurant RP" --other-network

# raport HTML și CSV în folderul curent
webrt-network-mapper export --format html --project "Restaurant RP" --author "Răzvan"
webrt-network-mapper export --format csv  --project 1

# instantaneu programat (ex. din cron, lunar)
webrt-network-mapper scan --yes --project "Restaurant RP" && \
webrt-network-mapper snapshot save --project "Restaurant RP" --name "$(date +%Y-%m) Restaurant RP"
```

`--yes` sare peste confirmarea de utilizare legitimă și permite scanarea subnet-urilor mai mari de /22.

---

## 6. Unde stau datele

| Ce | Linux | macOS | Windows |
|---|---|---|---|
| Baza de date (proiecte, inventar, hărți) | `~/.local/share/webrt-network-mapper/network-map.db` | `~/Library/Application Support/webrt-network-mapper/` | `%AppData%\webrt-network-mapper\` |
| Instantanee (fișiere `.netmap.json`) | `~/Documente/webrt-network-mapper/` | `~/Documents/webrt-network-mapper/` | `Documents\webrt-network-mapper\` |

- Variabila `NETMAP_DATA=/altă/cale` mută baza de date, util pentru teste sau pentru o copie separată.
- Backup complet: copiezi folderul bazei de date, sau exporți JSON-ul fiecărui proiect. Instantaneele sunt deja fișiere independente.
- Dacă rulezi din linia de comandă cu `sudo`, baza de date se creează tot în home-ul tău, nu în `/root`, și îți aparține. Oricum nu e nevoie de `sudo`: după `make install`, ARP raw merge prin `setcap`.

---

## 7. Scurtături tastatură

| Tastă | Acțiune |
|---|---|
| `V` `R` `W` `C` `T` `D` `P` `E` | Select · Cameră · Perete · Cablu · Text · Ușă · Survey · Șterge |
| `M` | cablu drept / ortogonal |
| `Shift` | constrângere 0/45/90° · `Shift`+click = adaugă la selecție · plasare multiplă |
| `Alt` | fără snap · `Alt`+click pe cablu = colț nou |
| `G` / `S` | grilă on/off · snap on/off |
| `F` | încadrează tot |
| `+` `-` `0` | zoom · rotița mouse-ului = zoom la cursor |
| `Space`+drag / click-mijloc | pan |
| Săgeți | mută selecția 1 pas de grilă (`Shift` = 5 pași) |
| `Del` | șterge selecția / colțul selectat |
| `Enter` / dublu-click | termină peretele sau cablul |
| `Esc` / click-dreapta | anulează operația curentă |
| `Ctrl+Z` / `Ctrl+Y` | undo / redo (200 de pași) |
| `Ctrl+C` `Ctrl+V` `Ctrl+D` `Ctrl+A` | copiere · lipire · duplicare · selectează tot |
| `Ctrl+S` | salvează harta |

---

## 8. Probleme frecvente

**„Altă rețea decât a locației” / CLI: „rețeaua curentă … nu a mai fost scanată în locația …”.**
Laptopul e conectat la un router pe care locația deschisă nu l-a mai văzut. Scanul a fost oprit ca să nu amestece inventarul a două locuri. Dacă ești în alt loc, alege locația corectă sau creează una nouă. Dacă e același loc cu un router în plus, alege „Scanează totuși” (CLI: `--other-network`). Vezi [Locații și etaje](#locații-și-etaje--ce-e-fiecare).

**`info` arată „ARP raw: indisponibil” și scanul spune „ping + tabela ARP”.**
Binarul nu are capabilitatea `cap_net_raw`. Rulezi:
```bash
sudo setcap cap_net_raw+ep "$(command -v webrt-network-mapper)"
```
Trebuie reaplicată după fiecare recompilare (`make install` o face automat). Fără ea, aplicația merge oricum, doar pe fallback.

**„nmcli nu este instalat” la scanul Wi-Fi.**
Pe Linux, Wi-Fi-ul se citește prin NetworkManager: `sudo apt install network-manager`. Laptopul trebuie să aibă placa Wi-Fi pornită. Dacă ești conectat prin cablu, Wi-Fi-ul trebuie doar să fie activ, nu conectat.

**„nu pot asculta pe 127.0.0.1:8765”.**
Portul e ocupat de alt program. Pornești pe alt port cu `webrt-network-mapper web --addr 127.0.0.1:9000`.

**Unele telefoane nu apar la scan.**
Telefoanele în sleep nu răspund la ping. Cu ARP raw (setcap), rata de detecție e mai bună. Mai încearcă un scan cu telefonul deblocat. Telefoanele moderne folosesc **MAC privat (aleator)** per rețea, marcat ca atare în coloana Vendor.

**„Harta a fost modificată în altă parte”.**
Harta e deschisă în 2 tab-uri. Alegi „Reîncarcă versiunea salvată” sau „Suprascrie cu versiunea mea”.

**Asocierea BSSID → router e greșită.**
E o estimare (status *inferat*) pe baza MAC-urilor apropiate numeric. O respingi din tab-ul Wi-Fi și alegi manual echipamentul corect din listă.

---

## 9. Arhitectură

```
browser (127.0.0.1:8765) ──HTTP/JSON──► net/http (internal/api)
                                            │
             ┌──────────────────────────────┼──────────────────────────────┐
             ▼                              ▼                              ▼
   internal/jobs (scanări)        internal/export (CSV/JSON/SVG/HTML)   internal/snapshot
     │            │                         │                            (fișiere .netmap.json,
     ▼            ▼                         ▼                             comparare)
 scanner/     wifi/                   internal/db ◄──────────────────────────┘
 (ARP, ping,  (nmcli / netsh /        SQLite + migrații (embed)
  porturi,     system_profiler)
  OUI, tip)
```

**Model de date** (SQLite, `internal/db/migrations`):

```
projects ─┬─< scans ─┬─< device_observations >─ devices   (UNIQUE project_id+mac)
          │          └─< wifi_observations   >─ wifi_networks (UNIQUE project_id+bssid)
          │                    └── survey_point_id ─► survey_points
          ├─< maps  (scene_json = toată harta, version = optimistic locking)
          └── settings (snapshot_dir, scan_consent)
```

- **Inventarul e legat de MAC.** La un rescan, upsert-ul actualizează IP-ul, hostname-ul, vendorul și tipul detectat, dar nu atinge niciodată `custom_name`, `device_type`, `location`, `notes`, `color`. Dispozitivele care nu răspund rămân în inventar ca „offline”.
- **Harta** e un singur JSON (`maps.scene_json`): camere, pereți, uși, noduri (cu `device_id`), cabluri (`from`/`to` = nod+port sau punct liber, `points` = colțuri, `route` = traseul calculat), texte, puncte survey, straturi, grilă, scară. Salvarea trimite `version`: dacă harta s-a schimbat între timp, serverul răspunde `409`.
- **Randarea SVG** (export, raport, comparare) se face pe server, în Go, din același `scene_json` și cu aceleași icon-uri (`web/icons.json`) ca editorul canvas. PNG-ul e SVG-ul rasterizat în browser.
- **Instantaneele** au același format ca backup-ul JSON (`format: "webrt-network-mapper"`). Căile primite din UI sunt verificate să nu iasă din folderul instantaneelor.

**Pașii scanării LAN** (`internal/scanner/lan.go`):
1. Detectează interfața rutei implicite, IP-ul, subnetul, gateway-ul și DNS-ul (inclusiv serverele reale din `systemd-resolved`).
2. ARP raw (`mdlayher/arp`, 2 runde) dacă are `CAP_NET_RAW`; altfel ping sweep (128 simultan, prin utilitarul `ping`) + tabela ARP a sistemului.
3. Pentru fiecare host, în paralel: vendor din OUI, hostname (reverse DNS → mDNS unicast → NetBIOS), TCP connect cu timeout 300 ms pe `21 22 23 53 80 139 443 445 515 554 631 1883 3389 5900 8080 8443 9100` (max 64 conexiuni simultane).
4. Clasificare (`classify.go`): gateway → router; 9100/515/631 → imprimantă; 554 / Hikvision / Dahua → cameră; TP-Link/Ubiquiti/Mikrotik cu web → AP sau router; 3389 → PC Windows; fără porturi + Apple/Samsung/Xiaomi sau MAC aleator → telefon.

**Deducerea AP-urilor** (`internal/wifi/infer.go`):
- Un BSSID e asociat cu dispozitivul LAN care are același OUI și MAC-ul la cel mult 8 distanță numerică.
- Pentru BSSID-urile administrate local (TP-Link: `7A:06:C3:…` pentru routerul `C0:06:C3:…`) se compară partea NIC.
- BSSID-urile care diferă doar la final sunt tratate ca „același echipament, radio 2.4/5 GHz”.

**Securitate:**
- Serverul ascultă doar pe 127.0.0.1.
- Cererile cu alt `Host` sunt respinse (protecție la DNS rebinding), la fel cele de modificare cu alt `Origin`.
- Nu se fac autentificări pe dispozitive și nu se stochează parole Wi-Fi.

---

## 10. API REST local

Folosit de interfață; util și pentru scripturi (`curl http://127.0.0.1:8765/api/...`).

| Metodă | Rută | Descriere |
|---|---|---|
| GET | `/api/info` | versiune, căi de date, rețeaua detectată |
| GET / POST | `/api/projects` | listă / creare `{"name","address","notes"}` |
| GET / PATCH / DELETE | `/api/projects/{id}` | detalii (+ ultimul scan LAN) / editare / ștergere |
| POST | `/api/projects/{id}/scan` | `{"lan":true,"wifi":true,"interface":"","subnet":"","allow_large":false,"allow_other_network":false}` → `{"scan_ids":[..]}`; `409 {"code":"other_network","details":{…}}` dacă routerul curent nu aparține locației |
| GET | `/api/scans/{id}` | status + progres (polling) |
| POST | `/api/scans/{id}/cancel` | oprește scanarea |
| GET / POST | `/api/projects/{id}/devices` | inventar / dispozitiv manual `{"mac","ip","name","type"}` |
| PATCH / DELETE | `/api/devices/{id}` | `custom_name, device_type, location, notes, color` / ștergere |
| GET | `/api/projects/{id}/wifi` | rețele Wi-Fi (cu ultimul semnal) |
| PATCH | `/api/wifi/{id}` | `label, location, notes, ap_device_id, ap_link_status (none/inferat/confirmat/respins)` |
| GET / POST | `/api/projects/{id}/survey-points` | listă / punct nou `{"label","x","y","map_id"}` (rulează scan Wi-Fi) |
| PATCH / DELETE | `/api/survey-points/{id}` | editare / ștergere |
| GET / POST | `/api/projects/{id}/maps` | hărți / hartă nouă |
| GET / PUT / PATCH / DELETE | `/api/maps/{id}` | citire / salvare `{"scene_json","version","background"}` (409 la conflict) / redenumire / ștergere |
| GET | `/api/projects/{id}/export?format=csv\|json\|html\|pdf` | export (`&inline=1` = afișare în browser) |
| POST | `/api/import?name=` | restaurare din backup JSON (corpul = fișierul) |
| POST | `/api/render/svg` | randează o scenă în SVG |
| GET / PUT | `/api/settings/snapshot-dir` | folderul instantaneelor |
| GET | `/api/snapshots` | listă instantanee |
| POST | `/api/projects/{id}/snapshots` | salvează `{"name","note","folder"}` |
| PATCH / DELETE | `/api/snapshots?path=` | redenumire `{"name"}` / ștergere |
| GET | `/api/snapshots/file?path=&download=1` · `/api/snapshots/report?path=` | fișierul / raportul unui instantaneu |
| POST | `/api/snapshots/restore` | `{"path","name"}` → proiect nou |
| POST | `/api/compare` | `{"a":"snap:<cale>" \| "current:<id>", "b":...}` → diferențe + SVG-urile hărților |

---

## 11. Referință rapidă fișiere

```
network-mapper/
├── cmd/webrt-network-mapper/
│   ├── main.go              # CLI: web | scan | export | import | snapshot | projects | info
│   ├── priv_unix.go         # sudo → fișierele rămân ale utilizatorului; status cap_net_raw
│   └── priv_windows.go
├── internal/
│   ├── config/config.go     # adresă server, folder date per OS, NETMAP_DATA
│   ├── db/
│   │   ├── db.go            # conexiune SQLite (WAL) + migrații embed
│   │   ├── store.go         # toate interogările (upsert după MAC/BSSID, optimistic locking)
│   │   └── migrations/*.sql
│   ├── scanner/
│   │   ├── lan.go           # orchestrarea scanului LAN
│   │   ├── iface.go         # interfață, subnet, gateway, DNS
│   │   ├── arp_linux.go     # ARP raw (mdlayher/arp) + /proc/net/arp
│   │   ├── arp_other.go     # `arp -a` pe Windows/macOS
│   │   ├── ping.go          # ping sweep concurent
│   │   ├── hostname.go      # reverse DNS, mDNS, NetBIOS
│   │   ├── ports.go         # TCP connect
│   │   ├── vendor.go        # lookup OUI (assets/oui.csv), MAC aleator
│   │   └── classify.go      # ghicirea tipului
│   ├── bluetooth/           # descoperire Bluetooth pasivă (bluetoothctl), ghicire tip
│   ├── wifi/
│   │   ├── wifi.go          # model comun, bandă/canal, % → dBm
│   │   ├── nmcli_linux.go · netsh_windows.go · airport_darwin.go
│   │   ├── connect_linux.go # conectare temporară cu parola utilizatorului + revenire
│   │   └── infer.go         # BSSID ↔ AP, perechi 2.4/5 GHz, avertismente canale
│   ├── jobs/jobs.go         # rulează scanările (fundal / CLI) și scrie în DB
│   ├── export/
│   │   ├── json.go          # backup + restaurare (remapare ID-uri în scene)
│   │   ├── csv.go
│   │   ├── svg.go           # randarea hărții în SVG (aceleași icon-uri ca editorul)
│   │   └── html.go          # raportul (html/template, print CSS pentru PDF)
│   ├── snapshot/
│   │   ├── snapshot.go      # salvare/listare/redenumire/ștergere .netmap.json
│   │   └── compare.go       # diferențe dispozitive, Wi-Fi, hărți
│   └── api/routes.go        # REST + servirea frontend-ului, protecție Host/Origin
├── assets/oui.csv           # registrul IEEE MA-L (≈40.000 producători), inclus în binar
├── web/                     # frontend inclus în binar (//go:embed)
│   ├── index.html · icons.json (cele 40 de icon-uri SVG)
│   ├── css/app.css · css/report.css
│   └── js/
│       ├── app.js           # bootstrap, proiecte/hărți, salvare, scan, taste
│       ├── autolayout.js    # harta generată automat din scan (Internet → router → coloane)
│       ├── api.js · ui.js · state.js
│       ├── mapview.js       # unelte, paletă, neplasate, panou proprietăți
│       ├── devices.js · wifi.js · survey.js · snapshots.js · export.js
│       └── editor/
│           ├── canvas.js    # randare, zoom/pan, selecție, clipboard, aliniere, minimap
│           ├── tools.js     # select, cameră, perete, cablu, text, ușă, survey, scară, radieră
│           ├── scene.js     # modelul scenei, rutare ortogonală, geometrie
│           ├── snap.js · history.js · icons.js
├── packaging/               # lansator .desktop + icon
├── tools/gen_icons.py       # regenerează web/icons.json
├── Makefile                 # build · install · install-user · uninstall · dist · test
└── network-mapper-spec-go.md  # specificația inițială
```

Pentru dezvoltare: `make run` compilează și pornește aplicația, iar `make test` rulează `go vet` și testele. Frontend-ul nu are build step, dar e inclus în binar, așa că orice modificare în `web/` cere recompilare.
