package service

import (
	"strings"
	"testing"
)

func tuningMap(items []tuningItem) map[string]string {
	m := make(map[string]string, len(items))
	for _, item := range items {
		m[item.Key] = item.Value
	}
	return m
}

func TestRecommendTuningTemplate(t *testing.T) {
	if got := recommendTuningTemplate(2); got != TuneTemplateTiny {
		t.Fatalf("2C: got %s", got)
	}
	if got := recommendTuningTemplate(4); got != TuneTemplateOLTP {
		t.Fatalf("4C: got %s", got)
	}
}

func TestPgTuningTiny(t *testing.T) {
	m := tuningMap(pgTuning(TuneTemplateTiny, tuningBasis{CPU: 2, MemMB: 4096, DiskGB: 80}, "16.15-alpine", false))
	expect := map[string]string{
		"max_connections":                 "250",
		"shared_buffers":                  "1024MB",
		"maintenance_work_mem":            "256MB",
		"effective_cache_size":            "3072MB",
		"work_mem":                        "16MB",
		"max_worker_processes":            "12",
		"max_parallel_workers":            "1",
		"max_parallel_workers_per_gather": "0",
		"min_wal_size":                    "4GB",
		"max_wal_size":                    "16GB",
		"autovacuum_max_workers":          "2",
		"wal_compression":                 "lz4",
	}
	for k, v := range expect {
		if m[k] != v {
			t.Errorf("%s: got %q want %q", k, m[k], v)
		}
	}
	if _, ok := m["io_method"]; ok {
		t.Errorf("io_method must only be set on PG18+")
	}
	if _, ok := m["parallel_setup_cost"]; ok {
		t.Errorf("parallel cost overrides are oltp only")
	}
}

func TestPgTuningOLTP(t *testing.T) {
	basis := tuningBasis{CPU: 8, MemMB: 32768, DiskGB: 500}
	m := tuningMap(pgTuning(TuneTemplateOLTP, basis, "18.6-alpine", false))
	expect := map[string]string{
		"max_connections":                  "1000",
		"shared_buffers":                   "8192MB",
		"work_mem":                         "64MB",
		"max_worker_processes":             "16",
		"max_parallel_workers":             "4",
		"max_parallel_workers_per_gather":  "2",
		"max_parallel_maintenance_workers": "3",
		"io_method":                        "worker",
		"io_workers":                       "4",
		"min_wal_size":                     "25GB",
		"max_wal_size":                     "100GB",
		"default_statistics_target":        "400",
	}
	for k, v := range expect {
		if m[k] != v {
			t.Errorf("%s: got %q want %q", k, m[k], v)
		}
	}
	if got := tuningMap(pgTuning(TuneTemplateOLTP, basis, "18.6-alpine", true))["max_connections"]; got != "500" {
		t.Errorf("pgbouncer max_connections: got %s", got)
	}
	if items := pgTuning(TuneTemplateNone, basis, "18.6-alpine", false); items != nil {
		t.Errorf("none template must not tune")
	}
}

func TestRedisTuning(t *testing.T) {
	tiny := tuningMap(redisTuning(TuneTemplateTiny, tuningBasis{CPU: 1, MemMB: 1024}, "8.10.2"))
	if tiny["maxmemory"] != "256mb" || tiny["appendonly"] != "no" || tiny["maxclients"] != "2000" {
		t.Errorf("tiny: %v", tiny)
	}
	if _, ok := tiny["io-threads"]; ok {
		t.Errorf("tiny must not set io-threads")
	}
	oltp := tuningMap(redisTuning(TuneTemplateOLTP, tuningBasis{CPU: 8, MemMB: 16384}, "7.4.11"))
	if oltp["maxmemory"] != "8192mb" || oltp["io-threads"] != "4" || oltp["appendonly"] != "yes" {
		t.Errorf("oltp: %v", oltp)
	}
	limited := tuningMap(redisTuning(TuneTemplateTiny, tuningBasis{CPU: 2, MemMB: 512, MemLimited: true}, "8.10.2"))
	if limited["maxmemory"] != "384mb" {
		t.Errorf("limited: %v", limited["maxmemory"])
	}
}

func TestMemoryLimitMB(t *testing.T) {
	if memoryLimitMB(2, "G") != 2048 || memoryLimitMB(512, "M") != 512 || memoryLimitMB(0, "M") != 0 {
		t.Fatal("memoryLimitMB conversion")
	}
}

func TestRenderPgTuningBlockReplacesPrevious(t *testing.T) {
	items := []tuningItem{{"shared_buffers", "256MB"}, {"random_page_cost", "1.1"}}
	first := renderPgTuningBlock("listen_addresses = '*'\n", items)
	second := renderPgTuningBlock(first, []tuningItem{{"shared_buffers", "512MB"}})
	if strings.Count(second, pgTuningBlockStart) != 1 || !strings.Contains(second, "shared_buffers = '512MB'") || strings.Contains(second, "random_page_cost") {
		t.Fatalf("unexpected block:\n%s", second)
	}
	if !strings.Contains(first, "random_page_cost = 1.1") || !strings.HasPrefix(second, "listen_addresses = '*'") {
		t.Fatalf("unexpected render:\n%s", first)
	}
}

func TestInjectPgBouncerService(t *testing.T) {
	services := map[string]interface{}{"pg": map[string]interface{}{}}
	injectPgBouncerService(services, "pg")
	svc := services[pgBouncerServiceName].(map[string]interface{})
	if svc["depends_on"].(map[string]interface{})["pg"] == nil {
		t.Fatal("pgbouncer must depend on the postgres service")
	}
	params := map[string]interface{}{TuneTemplateKey: TuneTemplateOLTP, "CONTAINER_NAME": "1Panel-postgresql-abcd", "PANEL_DB_ROOT_USER": "user_x"}
	ini := renderPgBouncerIni(params)
	for _, want := range []string{"* = host=1Panel-postgresql-abcd port=5432", "pool_mode                 = transaction", "auth_user                 = user_x", "default_pool_size         = 50"} {
		if !strings.Contains(ini, want) {
			t.Errorf("missing %q in\n%s", want, ini)
		}
	}
}
