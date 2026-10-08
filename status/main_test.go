package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ps(name string, critical bool, ok *bool) ProbeStatus {
	p := ProbeStatus{Name: name, Critical: critical}
	if ok != nil {
		p.Latest = &Sample{OK: *ok}
	}
	return p
}

func TestOverall(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name   string
		probes []ProbeStatus
		want   string
	}{
		{"all ok", []ProbeStatus{ps("self", true, &yes), ps("http", false, &yes)}, "operational"},
		{"not run yet", []ProbeStatus{ps("self", true, nil), ps("http", false, nil)}, "operational"},
		{"outside probe fails", []ProbeStatus{ps("self", true, &yes), ps("outbound", false, &no)}, "degraded"},
		{"self fails", []ProbeStatus{ps("self", true, &no), ps("http", false, &yes)}, "down"},
		{"self failure beats degraded", []ProbeStatus{ps("outbound", false, &no), ps("self", true, &no)}, "down"},
	}
	for _, c := range cases {
		if got := Overall(c.probes); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestStoreBoundedAndOrdered(t *testing.T) {
	s := NewStore([]Probe{{Name: "b"}, {Name: "a"}})
	for i := 0; i < historySize+30; i++ {
		s.Add("a", Sample{LatencyMs: int64(i), OK: true})
	}
	snap := s.Snapshot()
	if snap[0].Name != "b" || snap[1].Name != "a" {
		t.Fatalf("probe order changed: %v, %v", snap[0].Name, snap[1].Name)
	}
	if snap[0].Latest != nil {
		t.Error("probe without samples must have Latest=nil")
	}
	a := snap[1]
	if len(a.History) != historySize {
		t.Fatalf("history %d, want %d", len(a.History), historySize)
	}
	if a.Latest.LatencyMs != int64(historySize+29) || a.History[0].LatencyMs != 30 {
		t.Errorf("the oldest samples must be dropped: first=%d last=%d", a.History[0].LatencyMs, a.Latest.LatencyMs)
	}
}

func TestHTTPProbe(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/bad", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ok", 301) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	run := func(path string, timeout time.Duration) Sample {
		return runProbe(context.Background(), Probe{run: httpProbe(client, srv.URL+path)}, timeout)
	}
	if s := run("/ok", time.Second); !s.OK {
		t.Errorf("/ok must be OK: %+v", s)
	}
	if s := run("/bad", time.Second); s.OK || !strings.Contains(s.Detail, "503") {
		t.Errorf("/bad must fail with 503: %+v", s)
	}
	// A redirect is not success: the probe must not follow it silently.
	if s := run("/redir", time.Second); s.OK {
		t.Errorf("/redir must count as a failure: %+v", s)
	}
	if s := run("/slow", 50*time.Millisecond); s.OK {
		t.Errorf("/slow must time out: %+v", s)
	}
}

func TestTLSProbe(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	ok := runProbe(context.Background(), Probe{run: tlsProbe(addr, &tls.Config{RootCAs: pool, ServerName: "example.com"})}, 2*time.Second)
	if !ok.OK || !strings.Contains(ok.Detail, "days") {
		t.Errorf("a trusted certificate must be OK and report the days left: %+v", ok)
	}
	// Without the pool the test certificate is untrusted: it must fail, not pass.
	bad := runProbe(context.Background(), Probe{run: tlsProbe(addr, &tls.Config{})}, 2*time.Second)
	if bad.OK {
		t.Errorf("an untrusted certificate must fail: %+v", bad)
	}
}

func TestTCPProbe(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := strings.TrimPrefix(srv.URL, "http://")
	if s := runProbe(context.Background(), Probe{run: tcpProbe(addr)}, time.Second); !s.OK {
		t.Errorf("an open port must be OK: %+v", s)
	}
	srv.Close()
	if s := runProbe(context.Background(), Probe{run: tcpProbe(addr)}, time.Second); s.OK {
		t.Errorf("a closed port must fail: %+v", s)
	}
}

func newTestApp() *app {
	probes := []Probe{{Name: "self", Critical: true, Target: "x"}, {Name: "outbound", Target: "y"}}
	a := &app{store: NewStore(probes), started: time.Now().Add(-90 * time.Second), interval: 30 * time.Second}
	a.store.Add("self", Sample{OK: true})
	a.store.Add("outbound", Sample{OK: false, Detail: "i/o timeout"})
	return a
}

func TestStatusEndpoint(t *testing.T) {
	h := newTestApp().routes()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("Authorization", "secret") // must not leak out
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Status != "degraded" {
		t.Errorf("status %q, want degraded", got.Status)
	}
	if got.UptimeSeconds < 90 || got.Persistent {
		t.Errorf("wrong uptime/persistent: %+v", got)
	}
	if got.Instance.EdgeHeaders["X-Forwarded-For"] != "203.0.113.9" {
		t.Errorf("edge header missing: %v", got.Instance.EdgeHeaders)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Error("header outside the allowlist leaked into the response")
	}
}

func TestIndexAndNotFound(t *testing.T) {
	h := newTestApp().routes()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "status.ronggur.my.id") {
		t.Errorf("wrong response for /: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/not-found", nil))
	if rec.Code != 404 {
		t.Errorf("/not-found: %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != 405 {
		t.Errorf("POST: %d, want 405", rec.Code)
	}
}

func TestForceHTTPSAndHealthz(t *testing.T) {
	a := newTestApp()
	h := a.routes()

	req := httptest.NewRequest("GET", "/?a=1", nil)
	req.Host = "status.ronggur.my.id"
	req.Header.Set("X-Forwarded-Proto", "http")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 301 || rec.Header().Get("Location") != "https://status.ronggur.my.id/?a=1" {
		t.Errorf("wrong redirect: %d %s", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS missing on HTTPS")
	}

	before := a.requests.Load()
	req = httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Forwarded-Proto", "http")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Errorf("/healthz must not be redirected: %d", rec.Code)
	}
	if a.requests.Load() != before {
		t.Error("/healthz must not count as a request")
	}
}

func TestDeriveRegion(t *testing.T) {
	t.Setenv("REGION", "")
	if r, src := deriveRegion("wd-abc-us-east-1-0"); r != "us-east-1" || src != "hostname" {
		t.Errorf("from hostname: %s %s", r, src)
	}
	if r, _ := deriveRegion("instance-0"); r != "unknown" {
		t.Errorf("no hint: %s", r)
	}
	t.Setenv("REGION", "us-central-1")
	if r, src := deriveRegion("wd-abc-us-east-1-0"); r != "us-central-1" || src != "env REGION" {
		t.Errorf("env must win: %s %s", r, src)
	}
}

func TestEnvDuration(t *testing.T) {
	t.Setenv("D", "45")
	if got := envDuration("D", time.Second); got != 45*time.Second {
		t.Errorf("bare number = seconds: %s", got)
	}
	t.Setenv("D", "2m")
	if got := envDuration("D", time.Second); got != 2*time.Minute {
		t.Errorf("2m: %s", got)
	}
	t.Setenv("D", "-5")
	if got := envDuration("D", 7*time.Second); got != 7*time.Second {
		t.Errorf("negative must fall back: %s", got)
	}
}

func TestSelfProbeRunsInProcess(t *testing.T) {
	detail, err := selfProbe(http.HandlerFunc(healthzHandler))(context.Background())
	if err != nil || detail != "HTTP 200" {
		t.Fatalf("healthy handler: detail=%q err=%v", detail, err)
	}
	broken := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) })
	if _, err := selfProbe(broken)(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("broken handler: err=%v, want HTTP 500", err)
	}
}

func TestHasNameserver(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"valid", write("valid", "# comment\nnameserver 2001:4860:4860::6464\n"), true},
		{"comments only", write("comments", "# nameserver 1.1.1.1\nsearch example.com\n"), false},
		{"empty", write("empty", ""), false},
		{"missing", filepath.Join(dir, "missing"), false},
	}
	for _, c := range cases {
		if got := hasNameserver(c.path); got != c.want {
			t.Errorf("%s: hasNameserver = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestResolverKeepsSystemWhenConfigured(t *testing.T) {
	if newResolver(true, "[::1]:53") != net.DefaultResolver {
		t.Fatal("a configured system resolver must be left alone")
	}
}

func TestFallbackResolverQueriesFallbackServer(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	got := make(chan int, 1)
	go func() {
		buf := make([]byte, 512)
		if n, _, err := pc.ReadFrom(buf); err == nil {
			got <- n
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, _ = newResolver(false, pc.LocalAddr().String()).LookupHost(ctx, "status-test.invalid")
	select {
	case n := <-got:
		if n == 0 {
			t.Fatal("empty DNS query")
		}
	case <-time.After(time.Second):
		t.Fatal("the fallback server never received a query")
	}
}

func TestWaitForRoute(t *testing.T) {
	calls := 0
	flaky := func() error {
		calls++
		if calls < 3 {
			return errors.New("network is unreachable")
		}
		return nil
	}
	if !waitForRoute(context.Background(), flaky, time.Millisecond, 10) || calls != 3 {
		t.Fatalf("flaky route: calls=%d, want ready on the 3rd call", calls)
	}
	never := func() error { return errors.New("network is unreachable") }
	if waitForRoute(context.Background(), never, time.Millisecond, 3) {
		t.Fatal("a route that never comes up must report false, not hang or lie")
	}
}

func TestRouteAvailable(t *testing.T) {
	if err := routeAvailable("127.0.0.1:53"); err != nil {
		t.Fatalf("loopback must be routable in the test environment: %v", err)
	}
}

func TestOutboundDefaultIsIPv6(t *testing.T) {
	t.Setenv("PROBE_OUTBOUND_ADDR", "")
	host, _, err := net.SplitHostPort(loadConfig().outboundAddr)
	if err != nil {
		t.Fatal(err)
	}
	if a, err := netip.ParseAddr(host); err != nil || !a.Is6() {
		t.Fatalf("default outbound target %q must be an IPv6 literal: the platform has no IPv4 egress", host)
	}
}

func TestGoogleProbe(t *testing.T) {
	t.Setenv("PROBE_GOOGLE_URL", "")
	cfg := loadConfig()
	if cfg.googleURL != "https://www.google.com/generate_204" {
		t.Fatalf("default google URL = %q", cfg.googleURL)
	}
	var found *Probe
	for _, p := range buildProbes(cfg) {
		if p.Name == "google" {
			found = &p
		}
	}
	if found == nil {
		t.Fatal("buildProbes has no google probe")
	}
	if found.Critical {
		t.Fatal("google is an outside dependency: it must not make the page report down")
	}
	if found.Target != cfg.googleURL {
		t.Fatalf("target = %q, want %q", found.Target, cfg.googleURL)
	}
}
