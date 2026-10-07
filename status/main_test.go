package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{"semua ok", []ProbeStatus{ps("self", true, &yes), ps("http", false, &yes)}, "operational"},
		{"belum jalan", []ProbeStatus{ps("self", true, nil), ps("http", false, nil)}, "operational"},
		{"probe luar gagal", []ProbeStatus{ps("self", true, &yes), ps("outbound", false, &no)}, "degraded"},
		{"self gagal", []ProbeStatus{ps("self", true, &no), ps("http", false, &yes)}, "down"},
		{"self gagal menang atas degraded", []ProbeStatus{ps("outbound", false, &no), ps("self", true, &no)}, "down"},
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
		t.Fatalf("urutan probe berubah: %v, %v", snap[0].Name, snap[1].Name)
	}
	if snap[0].Latest != nil {
		t.Error("probe tanpa sampel harus Latest=nil")
	}
	a := snap[1]
	if len(a.History) != historySize {
		t.Fatalf("history %d, want %d", len(a.History), historySize)
	}
	if a.Latest.LatencyMs != int64(historySize+29) || a.History[0].LatencyMs != 30 {
		t.Errorf("yang dibuang harus sampel tertua: first=%d last=%d", a.History[0].LatencyMs, a.Latest.LatencyMs)
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
		t.Errorf("/ok harus OK: %+v", s)
	}
	if s := run("/bad", time.Second); s.OK || !strings.Contains(s.Detail, "503") {
		t.Errorf("/bad harus gagal dengan 503: %+v", s)
	}
	// Redirect bukan sukses: probe tidak boleh mengikutinya diam-diam.
	if s := run("/redir", time.Second); s.OK {
		t.Errorf("/redir harus dianggap gagal: %+v", s)
	}
	if s := run("/slow", 50*time.Millisecond); s.OK {
		t.Errorf("/slow harus timeout: %+v", s)
	}
}

func TestTLSProbe(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	ok := runProbe(context.Background(), Probe{run: tlsProbe(addr, &tls.Config{RootCAs: pool, ServerName: "example.com"})}, 2*time.Second)
	if !ok.OK || !strings.Contains(ok.Detail, "hari") {
		t.Errorf("sertifikat tepercaya harus OK dan melaporkan sisa hari: %+v", ok)
	}
	// Tanpa pool, sertifikat uji tidak dipercaya: harus gagal, bukan lolos.
	bad := runProbe(context.Background(), Probe{run: tlsProbe(addr, &tls.Config{})}, 2*time.Second)
	if bad.OK {
		t.Errorf("sertifikat tak dipercaya harus gagal: %+v", bad)
	}
}

func TestTCPProbe(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := strings.TrimPrefix(srv.URL, "http://")
	if s := runProbe(context.Background(), Probe{run: tcpProbe(addr)}, time.Second); !s.OK {
		t.Errorf("port terbuka harus OK: %+v", s)
	}
	srv.Close()
	if s := runProbe(context.Background(), Probe{run: tcpProbe(addr)}, time.Second); s.OK {
		t.Errorf("port tertutup harus gagal: %+v", s)
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
	req.Header.Set("Authorization", "rahasia") // tidak boleh ikut keluar
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON tidak valid: %v", err)
	}
	if got.Status != "degraded" {
		t.Errorf("status %q, want degraded", got.Status)
	}
	if got.UptimeSeconds < 90 || got.Persistent {
		t.Errorf("uptime/persistent salah: %+v", got)
	}
	if got.Instance.EdgeHeaders["X-Forwarded-For"] != "203.0.113.9" {
		t.Errorf("header edge hilang: %v", got.Instance.EdgeHeaders)
	}
	if strings.Contains(rec.Body.String(), "rahasia") {
		t.Error("header di luar allowlist bocor ke respons")
	}
}

func TestIndexAndNotFound(t *testing.T) {
	h := newTestApp().routes()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "status.ronggur.my.id") {
		t.Errorf("/ salah: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tidak-ada", nil))
	if rec.Code != 404 {
		t.Errorf("/tidak-ada: %d, want 404", rec.Code)
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
		t.Errorf("redirect salah: %d %s", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS hilang di HTTPS")
	}

	before := a.requests.Load()
	req = httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Forwarded-Proto", "http")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Errorf("/healthz tidak boleh di-redirect: %d", rec.Code)
	}
	if a.requests.Load() != before {
		t.Error("/healthz tidak boleh dihitung sebagai request")
	}
}

func TestDeriveRegion(t *testing.T) {
	t.Setenv("REGION", "")
	if r, src := deriveRegion("wd-abc-us-east-1-0"); r != "us-east-1" || src != "hostname" {
		t.Errorf("dari hostname: %s %s", r, src)
	}
	if r, _ := deriveRegion("instance-0"); r != "tidak diketahui" {
		t.Errorf("tanpa petunjuk: %s", r)
	}
	t.Setenv("REGION", "us-central-1")
	if r, src := deriveRegion("wd-abc-us-east-1-0"); r != "us-central-1" || src != "env REGION" {
		t.Errorf("env harus menang: %s %s", r, src)
	}
}

func TestEnvDuration(t *testing.T) {
	t.Setenv("D", "45")
	if got := envDuration("D", time.Second); got != 45*time.Second {
		t.Errorf("angka polos = detik: %s", got)
	}
	t.Setenv("D", "2m")
	if got := envDuration("D", time.Second); got != 2*time.Minute {
		t.Errorf("2m: %s", got)
	}
	t.Setenv("D", "-5")
	if got := envDuration("D", 7*time.Second); got != 7*time.Second {
		t.Errorf("negatif harus fallback: %s", got)
	}
}
