package scanner

import (
	"context"
	"net/netip"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var reLatency = regexp.MustCompile(`(?i)(?:time|timp)[=<]\s*([\d.,]+)\s*ms`)

// pingOne apelează utilitarul ping al sistemului (nu cere privilegii).
// Întoarce latența în ms sau -1 dacă host-ul nu răspunde.
func pingOne(ctx context.Context, ip netip.Addr) float64 {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", "800", ip.String())
	case "darwin":
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "800", ip.String())
	default:
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", ip.String())
	}
	hideWindow(cmd)
	start := time.Now()
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	// Pe Windows, „Destination host unreachable” are exit code 0; verificăm TTL.
	if runtime.GOOS == "windows" && !regexp.MustCompile(`(?i)TTL=`).Match(out) {
		return -1
	}
	if m := reLatency.FindSubmatch(out); m != nil {
		s := string(m[1])
		for i := range s {
			if s[i] == ',' {
				s = s[:i] + "." + s[i+1:]
			}
		}
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v
		}
	}
	return float64(time.Since(start).Microseconds()) / 1000
}

// pingSweep dă ping concurent (max 128 simultan) și întoarce host-urile care au răspuns.
func pingSweep(ctx context.Context, hosts []netip.Addr, progress func(done, total int)) map[netip.Addr]float64 {
	const workers = 128
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	alive := map[netip.Addr]float64{}
	var wg sync.WaitGroup
	done := 0
	for _, h := range hosts {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(h netip.Addr) {
			defer wg.Done()
			defer func() { <-sem }()
			lat := pingOne(ctx, h)
			mu.Lock()
			if lat >= 0 {
				alive[h] = lat
			}
			done++
			if progress != nil {
				progress(done, len(hosts))
			}
			mu.Unlock()
		}(h)
	}
	wg.Wait()
	return alive
}
