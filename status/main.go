// Status page for status.ronggur.my.id.
//
// The page reports on itself honestly, failures included: it is also a test
// case for Datum Compute, the ALB and DNS. Compute preview has no outbound
// internet, no logs, no probes and no persistent disk, so everything the
// process knows is observable over HTTP and history lives in memory only.
//
// Built with CGO_ENABLED=0 so the binary is static (FROM scratch image).
package main

import (
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed index.html
var indexHTML []byte

// version is set at build time: -ldflags "-X main.version=<sha>".
var version = "dev"

const historySize = 120

// Sample is one probe result.
type Sample struct {
	Time      time.Time `json:"time"`
	OK        bool      `json:"ok"`
	LatencyMs int64     `json:"latency_ms"`
	Detail    string    `json:"detail"`
}

// Probe is one named check run on every interval.
type Probe struct {
	Name string `json:"name"`
	// Critical probes failing means the page itself is down; the others only
	// degrade it (they depend on the outside world).
	Critical bool   `json:"critical"`
	Target   string `json:"target"`
	run      func(ctx context.Context) (string, error)
}

// ProbeStatus is a probe plus its history, newest last.
type ProbeStatus struct {
	Name     string   `json:"name"`
	Target   string   `json:"target"`
	Critical bool     `json:"critical"`
	Latest   *Sample  `json:"latest"`
	History  []Sample `json:"history"`
}

// Store keeps a bounded history per probe. Safe for concurrent use.
type Store struct {
	mu      sync.Mutex
	order   []string
	targets map[string]Probe
	history map[string][]Sample
}

func NewStore(probes []Probe) *Store {
	s := &Store{targets: map[string]Probe{}, history: map[string][]Sample{}}
	for _, p := range probes {
		s.order = append(s.order, p.Name)
		s.targets[p.Name] = p
	}
	return s
}

func (s *Store) Add(name string, sm Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := append(s.history[name], sm)
	if len(h) > historySize {
		h = h[len(h)-historySize:]
	}
	s.history[name] = h
}

func (s *Store) Snapshot() []ProbeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProbeStatus, 0, len(s.order))
	for _, name := range s.order {
		p := s.targets[name]
		h := append([]Sample(nil), s.history[name]...)
		ps := ProbeStatus{Name: name, Target: p.Target, Critical: p.Critical, History: h}
		if len(h) > 0 {
			l := h[len(h)-1]
			ps.Latest = &l
		}
		out = append(out, ps)
	}
	return out
}

// Overall derives the headline status from the latest sample of each probe.
// A probe that has not run yet is ignored, so boot does not flash "down".
func Overall(probes []ProbeStatus) string {
	status := "operational"
	for _, p := range probes {
		if p.Latest == nil || p.Latest.OK {
			continue
		}
		if p.Critical {
			return "down"
		}
		status = "degraded"
	}
	return status
}

// runProbe executes one probe and times it.
func runProbe(parent context.Context, p Probe, timeout time.Duration) Sample {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	start := time.Now()
	detail, err := p.run(ctx)
	sm := Sample{Time: start.UTC(), LatencyMs: time.Since(start).Milliseconds(), OK: err == nil, Detail: detail}
	if err != nil {
		sm.Detail = err.Error()
	}
	return sm
}

func httpProbe(client *http.Client, url string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", "status-ronggur-my-id/"+version)
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return fmt.Sprintf("HTTP %d", resp.StatusCode), nil
	}
}

func dnsProbe(hosts []string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		var parts []string
		for _, h := range hosts {
			addrs, err := net.DefaultResolver.LookupHost(ctx, h)
			if err != nil {
				return "", fmt.Errorf("%s: %w", h, err)
			}
			parts = append(parts, fmt.Sprintf("%s=%d alamat", h, len(addrs)))
		}
		return strings.Join(parts, ", "), nil
	}
}

func tlsProbe(addr string, cfg *tls.Config) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		d := tls.Dialer{Config: cfg}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return "", err
		}
		defer conn.Close()
		certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
		if len(certs) == 0 {
			return "", errors.New("tidak ada sertifikat")
		}
		left := time.Until(certs[0].NotAfter)
		if left <= 0 {
			return "", fmt.Errorf("sertifikat kedaluwarsa %s", certs[0].NotAfter.Format("2006-01-02"))
		}
		return fmt.Sprintf("berlaku sampai %s (%d hari)", certs[0].NotAfter.Format("2006-01-02"), int(left.Hours()/24)), nil
	}
}

// tcpProbe dials a literal IP, so a failure cannot be blamed on DNS.
func tcpProbe(addr string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return "", err
		}
		conn.Close()
		return "terhubung", nil
	}
}

func buildProbes(port string, cfg config) []Probe {
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return []Probe{
		{Name: "self", Critical: true, Target: "127.0.0.1:" + port + "/healthz", run: httpProbe(client, "http://127.0.0.1:"+port+"/healthz")},
		{Name: "dns", Target: strings.Join(cfg.dnsHosts, ", "), run: dnsProbe(cfg.dnsHosts)},
		{Name: "http", Target: cfg.httpURL, run: httpProbe(client, cfg.httpURL)},
		{Name: "tls", Target: cfg.tlsAddr, run: tlsProbe(cfg.tlsAddr, &tls.Config{MinVersion: tls.VersionTLS12})},
		{Name: "outbound", Target: cfg.outboundAddr, run: tcpProbe(cfg.outboundAddr)},
	}
}

type config struct {
	dnsHosts     []string
	httpURL      string
	tlsAddr      string
	outboundAddr string
	interval     time.Duration
	probeTimeout time.Duration
}

func loadConfig() config {
	return config{
		dnsHosts:     splitList(env("PROBE_DNS_HOSTS", "ronggur.my.id,status.ronggur.my.id")),
		httpURL:      env("PROBE_HTTP_URL", "https://ronggur.my.id/healthz"),
		tlsAddr:      env("PROBE_TLS_ADDR", "ronggur.my.id:443"),
		outboundAddr: env("PROBE_OUTBOUND_ADDR", "1.1.1.1:443"),
		interval:     envDuration("PROBE_INTERVAL", 30*time.Second),
		probeTimeout: envDuration("PROBE_TIMEOUT", 5*time.Second),
	}
}

var regionRe = regexp.MustCompile(`[a-z]{2}-[a-z]+-\d+`)

// deriveRegion is a best effort: placements share one template, so env cannot
// differ per region. REGION wins when set; otherwise look for a region name in
// the instance hostname. "tidak diketahui" is a legitimate, reported result.
func deriveRegion(hostname string) (region, source string) {
	if v := os.Getenv("REGION"); v != "" {
		return v, "env REGION"
	}
	if m := regionRe.FindString(hostname); m != "" {
		return m, "hostname"
	}
	return "tidak diketahui", "tidak ada petunjuk di env atau hostname"
}

type Instance struct {
	Hostname     string            `json:"hostname"`
	Region       string            `json:"region"`
	RegionSource string            `json:"region_source"`
	Addresses    []string          `json:"addresses"`
	EdgeHeaders  map[string]string `json:"edge_headers"`
}

// edgeHeaders are the only request headers echoed back. Values are JSON
// encoded and rendered with textContent by the page, never as HTML.
var edgeHeaders = []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Request-Id", "Via"}

func instanceInfo(r *http.Request) Instance {
	host, _ := os.Hostname()
	region, src := deriveRegion(host)
	in := Instance{Hostname: host, Region: region, RegionSource: src, EdgeHeaders: map[string]string{}}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() {
				in.Addresses = append(in.Addresses, ipn.IP.String())
			}
		}
	}
	for _, h := range edgeHeaders {
		if v := r.Header.Get(h); v != "" {
			in.EdgeHeaders[h] = v
		}
	}
	return in
}

type statusResponse struct {
	Now           time.Time     `json:"now"`
	Status        string        `json:"status"`
	Version       string        `json:"version"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	Requests      int64         `json:"requests"`
	IntervalSecs  int64         `json:"probe_interval_seconds"`
	Persistent    bool          `json:"persistent_history"`
	Instance      Instance      `json:"instance"`
	Probes        []ProbeStatus `json:"probes"`
}

type app struct {
	store    *Store
	started  time.Time
	requests atomic.Int64
	interval time.Duration
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/", a.handleIndex)
	return a.count(forceHTTPS(readOnly(mux)))
}

func (a *app) count(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The self probe polls /healthz; counting it would drown real traffic.
		if r.URL.Path != "/healthz" {
			a.requests.Add(1)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(indexHTML)
}

func (a *app) handleStatus(w http.ResponseWriter, r *http.Request) {
	probes := a.store.Snapshot()
	resp := statusResponse{
		Now:           time.Now().UTC(),
		Status:        Overall(probes),
		Version:       version,
		UptimeSeconds: int64(time.Since(a.started).Seconds()),
		Requests:      a.requests.Load(),
		IntervalSecs:  int64(a.interval.Seconds()),
		Persistent:    false,
		Instance:      instanceInfo(r),
		Probes:        probes,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(resp)
}

func readOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// forceHTTPS: same behaviour as server/main.go. TLS ends at the Datum edge, so
// the scheme only comes from X-Forwarded-Proto. /healthz is never redirected.
func forceHTTPS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-Forwarded-Proto") {
		case "http":
			if r.URL.Path != "/healthz" {
				http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
				return
			}
		case "https":
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) probeLoop(ctx context.Context, probes []Probe, cfg config) {
	round := func() {
		var wg sync.WaitGroup
		for _, p := range probes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a.store.Add(p.Name, runProbe(ctx, p, cfg.probeTimeout))
			}()
		}
		wg.Wait()
	}
	round()
	t := time.NewTicker(cfg.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			round()
		}
	}
}

func main() {
	port := env("PORT", "8080")
	cfg := loadConfig()
	probes := buildProbes(port, cfg)
	a := &app{store: NewStore(probes), started: time.Now(), interval: cfg.interval}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go a.probeLoop(context.Background(), probes, cfg)

	log.Printf("status %s on :%s, probes every %s", version, port, cfg.interval)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("listen: %v", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	// Bare number = seconds.
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	log.Printf("%s=%q tidak valid, pakai %s", key, v, fallback)
	return fallback
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
