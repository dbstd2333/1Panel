package service

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
)

func TestRenderGuardConfig(t *testing.T) {
	cfg := defaultGuardGlobalConfig()
	cfg.IPDeny = []string{"1.1.1.1"}
	cfg.GeoMode = "deny"
	cfg.Countries = []string{"RU"}
	websites := map[uint]model.Website{1: {Alias: "a"}, 2: {Alias: "b"}, 3: {Alias: "c"}}
	sites := []model.WebsiteGuard{
		{WebsiteID: 1, Enable: true, Mode: "block", IPDeny: "2.2.2.0/24\n1.1.1.1", GeoMode: "allow", Countries: "CN,HK"},
		{WebsiteID: 2, Enable: true, Mode: "monitor", GeoMode: "none"},
		{WebsiteID: 3, Enable: false},
	}
	render, countries := renderGuardConfig(cfg, websites, sites, true)
	if render.Detector == "" || len(render.Sites) != 2 {
		t.Fatalf("unexpected render: %+v", render)
	}
	a := render.Sites["a"]
	if strings.Join(a.IPDeny, ",") != "1.1.1.1,2.2.2.0/24" || a.GeoMode != "allow" || strings.Join(a.Countries, ",") != "CN,HK" {
		t.Errorf("site a: %+v", a)
	}
	if b := render.Sites["b"]; b.GeoMode != "deny" || strings.Join(b.Countries, ",") != "RU" {
		t.Errorf("site b should inherit the global region policy: %+v", b)
	}
	if strings.Join(countries, ",") != "CN,HK,RU" {
		t.Errorf("countries: %v", countries)
	}
	if _, err := json.Marshal(render); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeGuardIPs(t *testing.T) {
	if _, err := normalizeGuardIPs([]string{"10.0.0.0/8", "::1", " 1.2.3.4 ", ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeGuardIPs([]string{"1.2.3"}); err == nil {
		t.Fatal("expected invalid ip error")
	}
}

func TestRenderGuardGeoConfWithoutCountries(t *testing.T) {
	conf, err := renderGuardGeoConf(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conf), "geo $guard_country {") || !strings.Contains(string(conf), "192.168.0.0/16 LAN;") {
		t.Fatalf("unexpected geo conf:\n%s", conf)
	}
}

func TestHLLAccuracy(t *testing.T) {
	h := newHLL()
	for i := uint64(0); i < 100000; i++ {
		h.add(i)
		h.add(i) // duplicates must not count
	}
	if got := float64(h.count()); math.Abs(got-100000)/100000 > 0.03 {
		t.Fatalf("hll estimate too far off: %v", got)
	}
	small := newHLL()
	for i := uint64(0); i < 50; i++ {
		small.add(i)
	}
	if got := small.count(); got < 48 || got > 52 {
		t.Fatalf("small cardinality estimate: %d", got)
	}
}

func TestHistPercentile(t *testing.T) {
	hist := make([]int64, len(monitorRtBounds)+1)
	hist[0] = 90 // <=10ms
	hist[3] = 10 // 50-100ms
	if p := histPercentile(hist, 0.95); p <= 50 || p > 100 {
		t.Fatalf("p95: %v", p)
	}
	if p := histPercentile(make([]int64, len(hist)), 0.95); p != 0 {
		t.Fatalf("empty hist: %v", p)
	}
}

func TestMonitorAggTolerantDecode(t *testing.T) {
	var line monitorLine
	raw := `{"site":"a","minute":60,"req":3,"in":10,"out":20,"s2":3,"hist":[3,0,0,0,0,0,0,0,0,0,0],"uv":{},"ips":[1,2],"top":{"url":{"/":3},"err":[]},"slow":{}}`
	if err := json.Unmarshal([]byte(raw), &line); err != nil {
		t.Fatal(err)
	}
	agg := newMonitorAgg("a", 60)
	agg.add(line)
	agg.add(line)
	if agg.req != 6 || len(agg.ips) != 2 || len(agg.uv) != 0 || agg.top["url"]["/"] != 6 {
		t.Fatalf("unexpected agg: req=%d ips=%d uv=%d top=%v", agg.req, len(agg.ips), len(agg.uv), agg.top["url"])
	}
}

func TestGuardSiteToDTO(t *testing.T) {
	res := guardSiteToDTO(model.WebsiteGuard{IPAllow: "1.1.1.1\n\n2.2.2.2", Countries: "CN,"})
	if len(res.IPAllow) != 2 || len(res.Countries) != 1 {
		t.Fatalf("unexpected dto: %+v", res)
	}
	_ = dto.GuardSiteConfig{}
}
