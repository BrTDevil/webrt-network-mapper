package export

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
	"sync"

	"webrt-network-mapper/web"
)

// Scene e subsetul din scene_json necesar randării (vezi web/js/editor/scene.js).
type Scene struct {
	Grid struct {
		Size int `json:"size"`
	} `json:"grid"`
	Scale struct {
		UnitsPerMeter float64 `json:"unitsPerMeter"`
	} `json:"scale"`
	Layers     []Layer      `json:"layers"`
	Background *Background  `json:"background"`
	Rooms      []Room       `json:"rooms"`
	Walls      []Wall       `json:"walls"`
	Doors      []Door       `json:"doors"`
	Nodes      []Node       `json:"nodes"`
	Links      []Link       `json:"links"`
	Texts      []Text       `json:"texts"`
	Survey     []SurveyNode `json:"surveyPoints"`
}

type Layer struct {
	ID      string `json:"id"`
	Visible bool   `json:"visible"`
}
type Background struct {
	X, Y, W, H float64
	Opacity    float64 `json:"opacity"`
}
type Room struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	Color string  `json:"color"`
}
type Wall struct {
	Points    [][2]float64 `json:"points"`
	Thickness float64      `json:"thickness"`
}
type Door struct {
	Kind     string  `json:"kind"` // door | window
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	Rotation float64 `json:"rotation"`
}
type Node struct {
	ID       string  `json:"id"`
	Icon     string  `json:"icon"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Rotation float64 `json:"rotation"`
	Scale    float64 `json:"scale"`
	Label    string  `json:"label"`
	Sublabel string  `json:"sublabel"`
	DeviceID *int64  `json:"device_id"`
	BtID     *int64  `json:"bt_id"`
	Color    string  `json:"color"`
}
type Link struct {
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	Label  string       `json:"label"`
	Status string       `json:"status"`
	Route  [][2]float64 `json:"route"`
}
type Text struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Text  string  `json:"text"`
	Size  float64 `json:"size"`
	Color string  `json:"color"`
	Bold  bool    `json:"bold"`
}
type SurveyNode struct {
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
	SurveyPointID int64   `json:"survey_point_id"`
	Label         string  `json:"label"`
}

// CableStyles: culoare, grosime, model de linie (identic cu editorul).
var CableStyles = map[string]struct {
	Label, Color, Dash string
}{
	"ethernet":  {"Ethernet", "#1a73e8", ""},
	"fibra":     {"Fibră", "#f57c00", ""},
	"wifi":      {"Wi-Fi / wireless", "#2e9d4f", "2 5"},
	"poe":       {"Alimentare / PoE", "#d93025", "9 5"},
	"bluetooth": {"Bluetooth", "#7b1fa2", "1 5"},
	"inferat":   {"Necunoscut / inferat", "#80868b", "2 5"},
}

// IconDef e un icon din web/icons.json.
type IconDef struct {
	Label string `json:"label"`
	Cat   string `json:"cat"`
	SVG   string `json:"svg"`
}

var (
	iconsOnce sync.Once
	icons     struct {
		Categories []string           `json:"categories"`
		Icons      map[string]IconDef `json:"icons"`
	}
)

// Icons întoarce setul de icon-uri comun cu frontend-ul.
func Icons() map[string]IconDef {
	iconsOnce.Do(func() {
		b, err := web.FS.ReadFile("icons.json")
		if err == nil {
			json.Unmarshal(b, &icons)
		}
	})
	return icons.Icons
}

// RenderContext oferă informații din inventar pentru randare.
type RenderContext struct {
	Online        map[int64]bool   // device_id → online
	SurveyBest    map[int64]string // survey_point_id → „RP -52 dBm”
	BackgroundURI string
	Legend        bool
}

// ParseScene decodează scene_json.
func ParseScene(js string) (*Scene, error) {
	var s Scene
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Scene) visible(layer string) bool {
	for _, l := range s.Layers {
		if l.ID == layer {
			return l.Visible
		}
	}
	return true
}

// Bounds calculează dreptunghiul care cuprinde toată scena.
func (s *Scene) Bounds() (minX, minY, maxX, maxY float64) {
	minX, minY, maxX, maxY = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	add := func(x, y float64) {
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x), math.Max(maxY, y)
	}
	for _, r := range s.Rooms {
		add(r.X, r.Y)
		add(r.X+r.W, r.Y+r.H)
	}
	for _, w := range s.Walls {
		for _, p := range w.Points {
			add(p[0], p[1])
		}
	}
	for _, n := range s.Nodes {
		sc := nz(n.Scale, 1)
		add(n.X-26*sc-30, n.Y-26*sc)
		add(n.X+26*sc+30, n.Y+26*sc+30)
	}
	for _, l := range s.Links {
		for _, p := range l.Route {
			add(p[0], p[1])
		}
	}
	for _, t := range s.Texts {
		add(t.X, t.Y-nz(t.Size, 14))
		add(t.X+float64(len([]rune(t.Text)))*nz(t.Size, 14)*0.6, t.Y+4)
	}
	for _, sp := range s.Survey {
		add(sp.X-10, sp.Y-10)
		add(sp.X+90, sp.Y+30)
	}
	if s.Background != nil && s.Background.W > 0 {
		add(s.Background.X, s.Background.Y)
		add(s.Background.X+s.Background.W, s.Background.Y+s.Background.H)
	}
	if math.IsInf(minX, 1) {
		return 0, 0, 800, 600
	}
	return
}

// RenderSVG produce harta vectorială completă, pe fundal alb, cu legendă opțională.
func RenderSVG(s *Scene, rc RenderContext) string {
	minX, minY, maxX, maxY := s.Bounds()
	const pad = 40
	minX, minY = minX-pad, minY-pad
	maxX, maxY = maxX+pad, maxY+pad
	w, h := maxX-minX, maxY-minY
	legendH := 0.0
	var usedIcons []string
	var usedCables []string
	if rc.Legend {
		seen := map[string]bool{}
		for _, n := range s.Nodes {
			if !seen[n.Icon] {
				seen[n.Icon] = true
				usedIcons = append(usedIcons, n.Icon)
			}
		}
		sort.Strings(usedIcons)
		seenC := map[string]bool{}
		for _, l := range s.Links {
			t := cableType(l)
			if !seenC[t] {
				seenC[t] = true
				usedCables = append(usedCables, t)
			}
		}
		sort.Strings(usedCables)
		perRow := int(math.Max(1, math.Floor(w/210)))
		rows := int(math.Ceil(float64(len(usedIcons)+len(usedCables)) / float64(perRow)))
		legendH = 40 + float64(rows)*34
		if w < 640 {
			// Legenda are nevoie de lățime minimă.
			extra := 640 - w
			minX -= extra / 2
			w = 640
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s" width="%s" height="%s" font-family="system-ui, -apple-system, Segoe UI, Roboto, sans-serif">`,
		f(minX), f(minY), f(w), f(h+legendH), f(w), f(h+legendH))
	fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" fill="#ffffff"/>`, f(minX), f(minY), f(w), f(h+legendH))

	if s.visible("background") && s.Background != nil && rc.BackgroundURI != "" && s.Background.W > 0 {
		fmt.Fprintf(&b, `<image href="%s" x="%s" y="%s" width="%s" height="%s" opacity="%s" preserveAspectRatio="none"/>`,
			html.EscapeString(rc.BackgroundURI), f(s.Background.X), f(s.Background.Y), f(s.Background.W), f(s.Background.H), f(nz(s.Background.Opacity, 0.5)))
	}

	if s.visible("walls") {
		for _, r := range s.Rooms {
			c := r.Color
			if c == "" {
				c = "#e8f0fe"
			}
			fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" fill="%s" fill-opacity="0.55" stroke="%s" stroke-width="1.5"/>`,
				f(r.X), f(r.Y), f(r.W), f(r.H), esc(c), esc(c))
			if r.Name != "" {
				fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" dominant-baseline="middle" font-size="18" font-weight="600" fill="#5f6368" fill-opacity="0.75">%s</text>`,
					f(r.X+r.W/2), f(r.Y+r.H/2), esc(r.Name))
			}
		}
		for _, wl := range s.Walls {
			if len(wl.Points) < 2 {
				continue
			}
			fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="#3c4043" stroke-width="%s" stroke-linejoin="miter" stroke-linecap="square"/>`,
				pts(wl.Points), f(nz(wl.Thickness, 8)))
		}
		for _, d := range s.Doors {
			b.WriteString(doorSVG(d))
		}
	}

	if s.visible("links") {
		for _, l := range s.Links {
			if len(l.Route) < 2 {
				continue
			}
			st := CableStyles[cableType(l)]
			dash := ""
			if st.Dash != "" {
				dash = ` stroke-dasharray="` + st.Dash + `"`
			}
			fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"%s/>`,
				pts(l.Route), st.Color, dash)
			if l.Label != "" {
				mx, my := routeMid(l.Route)
				fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" font-size="11" fill="%s" stroke="#fff" stroke-width="3" paint-order="stroke">%s</text>`,
					f(mx), f(my-6), st.Color, esc(l.Label))
			}
		}
	}

	if s.visible("nodes") {
		ic := Icons()
		for _, n := range s.Nodes {
			sc := nz(n.Scale, 1)
			col := n.Color
			if col == "" {
				col = "#1a73e8"
			}
			def, ok := ic[n.Icon]
			if !ok {
				def = ic["necunoscut"]
			}
			size := 48 * sc
			fmt.Fprintf(&b, `<g transform="translate(%s %s) rotate(%s) translate(%s %s) scale(%s)" color="%s">%s</g>`,
				f(n.X), f(n.Y), f(n.Rotation), f(-size/2), f(-size/2), f(sc), esc(col), def.SVG)
			if n.DeviceID != nil && rc.Online != nil {
				c := "#9aa0a6"
				if rc.Online[*n.DeviceID] {
					c = "#1e8e3e"
				}
				fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="5" fill="%s" stroke="#fff" stroke-width="1.5"/>`, f(n.X+size/2-2), f(n.Y-size/2+2), c)
			}
			if s.visible("labels") {
				y := n.Y + size/2 + 14
				if n.Label != "" {
					fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" font-size="12" font-weight="600" fill="#202124" stroke="#fff" stroke-width="3" paint-order="stroke">%s</text>`,
						f(n.X), f(y), esc(n.Label))
					y += 14
				}
				if n.Sublabel != "" {
					fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" font-size="10.5" fill="#5f6368" stroke="#fff" stroke-width="3" paint-order="stroke">%s</text>`,
						f(n.X), f(y), esc(n.Sublabel))
				}
			}
		}
	}

	if s.visible("survey") {
		for _, sp := range s.Survey {
			fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="8" fill="#9334e6" fill-opacity=".2" stroke="#9334e6" stroke-width="2"/><circle cx="%s" cy="%s" r="2.5" fill="#9334e6"/>`,
				f(sp.X), f(sp.Y), f(sp.X), f(sp.Y))
			txt := sp.Label
			if best := rc.SurveyBest[sp.SurveyPointID]; best != "" {
				if txt != "" {
					txt += " · "
				}
				txt += best
			}
			if txt != "" {
				fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="11" fill="#7627bb" stroke="#fff" stroke-width="3" paint-order="stroke">%s</text>`,
					f(sp.X+12), f(sp.Y+4), esc(txt))
			}
		}
	}

	if s.visible("labels") {
		for _, t := range s.Texts {
			c := t.Color
			if c == "" {
				c = "#202124"
			}
			weight := "400"
			if t.Bold {
				weight = "700"
			}
			fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%s" font-weight="%s" fill="%s">%s</text>`,
				f(t.X), f(t.Y), f(nz(t.Size, 14)), weight, esc(c), esc(t.Text))
		}
	}

	if s.Scale.UnitsPerMeter > 0 {
		b.WriteString(rulerSVG(minX+16, maxY-16, s.Scale.UnitsPerMeter))
	}

	if rc.Legend && len(usedIcons)+len(usedCables) > 0 {
		b.WriteString(legendSVG(minX, maxY, w, usedIcons, usedCables))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func cableType(l Link) string {
	if _, ok := CableStyles[l.Type]; ok {
		return l.Type
	}
	return "inferat"
}

func doorSVG(d Door) string {
	w := nz(d.W, 40)
	if d.Kind == "window" {
		return fmt.Sprintf(`<g transform="translate(%s %s) rotate(%s)"><rect x="%s" y="-5" width="%s" height="10" fill="#fff" stroke="#3c4043" stroke-width="1.5"/><line x1="%s" y1="0" x2="%s" y2="0" stroke="#3c4043" stroke-width="1.5"/></g>`,
			f(d.X), f(d.Y), f(d.Rotation), f(-w/2), f(w), f(-w/2), f(w/2))
	}
	return fmt.Sprintf(`<g transform="translate(%s %s) rotate(%s)"><rect x="%s" y="-5" width="%s" height="10" fill="#fff"/><path d="M%s 0 L%s %s M%s %s A%s %s 0 0 1 %s 0" fill="none" stroke="#3c4043" stroke-width="1.5"/></g>`,
		f(d.X), f(d.Y), f(d.Rotation), f(-w/2), f(w), f(-w/2), f(-w/2), f(-w), f(-w/2), f(-w), f(w), f(w), f(w/2))
}

func rulerSVG(x, y, upm float64) string {
	// Alege o lungime „rotundă” de aproximativ 120 de unități.
	meters := 1.0
	for _, m := range []float64{0.5, 1, 2, 5, 10, 20, 50} {
		if m*upm <= 160 {
			meters = m
		}
	}
	l := meters * upm
	return fmt.Sprintf(`<g stroke="#3c4043" stroke-width="2"><line x1="%s" y1="%s" x2="%s" y2="%s"/><line x1="%s" y1="%s" x2="%s" y2="%s"/><line x1="%s" y1="%s" x2="%s" y2="%s"/></g><text x="%s" y="%s" font-size="11" fill="#3c4043" text-anchor="middle">%s m</text>`,
		f(x), f(y), f(x+l), f(y), f(x), f(y-5), f(x), f(y+5), f(x+l), f(y-5), f(x+l), f(y+5), f(x+l/2), f(y-8), trimFloat(meters))
}

func legendSVG(x, y, w float64, iconKeys, cables []string) string {
	var b strings.Builder
	ic := Icons()
	fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#dadce0"/>`, f(x+16), f(y+8), f(x+w-16), f(y+8))
	fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="13" font-weight="700" fill="#202124">Legendă</text>`, f(x+16), f(y+28))
	perRow := int(math.Max(1, math.Floor(w/210)))
	i := 0
	pos := func() (float64, float64) {
		col, row := i%perRow, i/perRow
		i++
		return x + 16 + float64(col)*210, y + 40 + float64(row)*34
	}
	for _, k := range iconKeys {
		px, py := pos()
		d, ok := ic[k]
		if !ok {
			d = ic["necunoscut"]
		}
		fmt.Fprintf(&b, `<g transform="translate(%s %s) scale(0.55)" color="#1a73e8">%s</g><text x="%s" y="%s" font-size="11.5" fill="#3c4043">%s</text>`,
			f(px), f(py), d.SVG, f(px+34), f(py+17), esc(d.Label))
	}
	for _, c := range cables {
		px, py := pos()
		st := CableStyles[c]
		dash := ""
		if st.Dash != "" {
			dash = ` stroke-dasharray="` + st.Dash + `"`
		}
		fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="3"%s/><text x="%s" y="%s" font-size="11.5" fill="#3c4043">Cablu: %s</text>`,
			f(px), f(py+13), f(px+26), f(py+13), st.Color, dash, f(px+34), f(py+17), esc(st.Label))
	}
	return b.String()
}

func routeMid(r [][2]float64) (float64, float64) {
	total := 0.0
	for i := 1; i < len(r); i++ {
		total += math.Hypot(r[i][0]-r[i-1][0], r[i][1]-r[i-1][1])
	}
	half := total / 2
	for i := 1; i < len(r); i++ {
		seg := math.Hypot(r[i][0]-r[i-1][0], r[i][1]-r[i-1][1])
		if half <= seg && seg > 0 {
			t := half / seg
			return r[i-1][0] + (r[i][0]-r[i-1][0])*t, r[i-1][1] + (r[i][1]-r[i-1][1])*t
		}
		half -= seg
	}
	return r[0][0], r[0][1]
}

func pts(p [][2]float64) string {
	s := make([]string, len(p))
	for i, q := range p {
		s[i] = f(q[0]) + "," + f(q[1])
	}
	return strings.Join(s, " ")
}

func f(v float64) string { return trimFloat(math.Round(v*100) / 100) }

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}

func nz(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

func esc(s string) string { return html.EscapeString(s) }
