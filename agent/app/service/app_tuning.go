package service

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/app/dto/response"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/psutil"
	"github.com/shirou/gopsutil/v4/mem"
)

// Install-time tuning for PostgreSQL and Redis. The PostgreSQL formulas are
// ported from the pigsty tiny (1-3C) and oltp (4C+) patroni templates, with
// the HA, replication, ssl and logging-collector parts left out because a
// 1Panel install is a single container. Pigsty has no redis tiers, so the two
// redis templates are defined here along the same lines.

const (
	TuneTemplateKey      = "PANEL_TUNE_TEMPLATE"
	PgBouncerEnableKey   = "PANEL_PGBOUNCER_ENABLE"
	PgBouncerPortKey     = "PANEL_APP_PORT_PGBOUNCER"
	PgBouncerPoolModeKey = "PANEL_PGBOUNCER_POOL_MODE"

	TuneTemplateNone = "none"
	TuneTemplateTiny = "tiny"
	TuneTemplateOLTP = "oltp"
)

type tuningBasis struct {
	CPU        int  `json:"cpu"`
	MemMB      int  `json:"memMB"`
	DiskGB     int  `json:"diskGB"`
	MemLimited bool `json:"memLimited"`
}

type tuningItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func isTuningApp(key string) bool {
	return key == "postgresql" || key == "redis"
}

func recommendTuningTemplate(cpu int) string {
	if cpu >= 4 {
		return TuneTemplateOLTP
	}
	return TuneTemplateTiny
}

func normalizeTuningTemplate(tpl string) string {
	switch tpl {
	case TuneTemplateTiny, TuneTemplateOLTP:
		return tpl
	default:
		return TuneTemplateNone
	}
}

// loadTuningBasis sizes the template against the container limits when they
// are set, falling back to the host resources. Limits only apply to installs
// with advanced settings, matching addDockerComposeCommonParam.
func loadTuningBasis(cfg request.AppContainerConfig) tuningBasis {
	basis := tuningBasis{CPU: 1, MemMB: 1024, DiskGB: 20}
	if cores, err := psutil.CPUInfo.GetLogicalCores(false); err == nil && cores > 0 {
		basis.CPU = cores
	} else if n := psutil.CPU.NumCPU(); n > 0 {
		basis.CPU = n
	}
	if vm, err := mem.VirtualMemory(); err == nil && vm.Total > 0 {
		basis.MemMB = int(vm.Total / 1024 / 1024)
	}
	if usage, err := psutil.DISK.GetUsage(global.Dir.AppInstallDir, false); err == nil && usage.Total > 0 {
		basis.DiskGB = int(usage.Total / 1024 / 1024 / 1024)
	}
	if !cfg.Advanced {
		return basis
	}
	if cfg.CpuQuota > 0 {
		if quota := int(math.Ceil(cfg.CpuQuota)); quota < basis.CPU {
			basis.CPU = quota
		}
	}
	if limitMB := memoryLimitMB(cfg.MemoryLimit, cfg.MemoryUnit); limitMB > 0 && limitMB < basis.MemMB {
		basis.MemMB = limitMB
		basis.MemLimited = true
	}
	return basis
}

func memoryLimitMB(limit float64, unit string) int {
	if limit <= 0 {
		return 0
	}
	switch strings.ToUpper(unit) {
	case "K":
		return int(limit / 1024)
	case "G":
		return int(limit * 1024)
	default:
		return int(limit)
	}
}

func appMajorVersion(version string) int {
	major := version
	if idx := strings.IndexAny(major, ".-"); idx > 0 {
		major = major[:idx]
	}
	v, _ := strconv.Atoi(major)
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func pgMaxConnections(tpl string, pgbouncer bool) int {
	if tpl == TuneTemplateTiny {
		return 250
	}
	if pgbouncer {
		return 500
	}
	return 1000
}

func pgTuning(tpl string, basis tuningBasis, version string, pgbouncer bool) []tuningItem {
	tpl = normalizeTuningTemplate(tpl)
	if tpl == TuneTemplateNone {
		return nil
	}
	oltp := tpl == TuneTemplateOLTP
	pgVersion := appMajorVersion(version)
	cpu := maxInt(basis.CPU, 1)

	maxConn := pgMaxConnections(tpl, pgbouncer)
	sharedBuffers := int(math.Ceil(float64(basis.MemMB) * 0.25))
	maintenanceMem := int(math.Ceil(float64(sharedBuffers) * 0.25))
	effectiveCache := basis.MemMB - sharedBuffers
	diskTwentieth := clampInt(int(math.Ceil(float64(basis.DiskGB)/20)), 1, 100)

	var workMem, workerProcesses, parallelWorkers, perGather, maintenanceWorkers, ioWorkers int
	if oltp {
		workMem = clampInt(sharedBuffers/maxConn, 64, 1024)
		workerProcesses = maxInt(cpu+8, 16)
		parallelWorkers = maxInt(int(math.Ceil(float64(cpu)*0.5)), 2)
		perGather = minInt(maxInt(int(math.Floor(float64(cpu)*0.2)), 2), 8)
		maintenanceWorkers = maxInt(int(math.Ceil(float64(cpu)*0.33)), 2)
		ioWorkers = clampInt(int(math.Ceil(float64(cpu)*0.25)), 4, 16)
	} else {
		workMem = clampInt(sharedBuffers/maxConn, 16, 256)
		workerProcesses = maxInt(cpu+4, 12)
		parallelWorkers = maxInt(int(math.Floor(float64(cpu)*0.5)), 1)
		perGather = 0
		maintenanceWorkers = maxInt(int(math.Floor(float64(cpu)*0.33)), 1)
		ioWorkers = 3
	}

	items := []tuningItem{
		{"max_connections", strconv.Itoa(maxConn)},
		{"max_worker_processes", strconv.Itoa(workerProcesses)},
	}
	if pgVersion >= 15 {
		items = append(items, tuningItem{"wal_compression", "lz4"})
	}
	items = append(items,
		tuningItem{"shared_buffers", fmt.Sprintf("%dMB", sharedBuffers)},
		tuningItem{"maintenance_work_mem", fmt.Sprintf("%dMB", maintenanceMem)},
		tuningItem{"work_mem", fmt.Sprintf("%dMB", workMem)},
		tuningItem{"max_parallel_workers", strconv.Itoa(parallelWorkers)},
		tuningItem{"max_parallel_maintenance_workers", strconv.Itoa(maintenanceWorkers)},
		tuningItem{"max_parallel_workers_per_gather", strconv.Itoa(perGather)},
	)
	if oltp {
		items = append(items,
			tuningItem{"parallel_setup_cost", "2000"},
			tuningItem{"parallel_tuple_cost", "0.2"},
			tuningItem{"min_parallel_table_scan_size", "32MB"},
			tuningItem{"min_parallel_index_scan_size", "2MB"},
		)
	}
	if pgVersion >= 13 {
		items = append(items, tuningItem{"hash_mem_multiplier", "8.0"})
	}
	items = append(items, tuningItem{"huge_pages", "try"})
	if pgVersion >= 18 {
		// io_uring is often blocked by the container seccomp profile.
		items = append(items, tuningItem{"io_method", "worker"})
		if pgVersion == 18 {
			items = append(items, tuningItem{"io_workers", strconv.Itoa(ioWorkers)})
		}
	}
	items = append(items,
		tuningItem{"temp_file_limit", fmt.Sprintf("%dGB", minInt(diskTwentieth, 200))},
		tuningItem{"vacuum_cost_delay", "20ms"},
		tuningItem{"vacuum_cost_limit", "2000"},
		tuningItem{"bgwriter_delay", "10ms"},
		tuningItem{"bgwriter_lru_maxpages", "800"},
		tuningItem{"bgwriter_lru_multiplier", "5.0"},
		tuningItem{"min_wal_size", fmt.Sprintf("%dGB", minInt(diskTwentieth, 200))},
		tuningItem{"max_wal_size", fmt.Sprintf("%dGB", minInt(diskTwentieth*4, 2000))},
		tuningItem{"wal_buffers", "16MB"},
		tuningItem{"wal_writer_delay", "20ms"},
		tuningItem{"wal_writer_flush_after", "1MB"},
		tuningItem{"commit_delay", "20"},
		tuningItem{"commit_siblings", "10"},
		tuningItem{"checkpoint_timeout", "15min"},
		tuningItem{"checkpoint_completion_target", "0.95"},
		tuningItem{"random_page_cost", "1.1"},
		tuningItem{"effective_io_concurrency", "200"},
	)
	if pgVersion >= 13 {
		items = append(items, tuningItem{"maintenance_io_concurrency", "100"})
	}
	statisticsTarget, autovacuumWorkers := "200", "2"
	if oltp {
		statisticsTarget, autovacuumWorkers = "400", "3"
	}
	items = append(items,
		tuningItem{"effective_cache_size", fmt.Sprintf("%dMB", effectiveCache)},
		tuningItem{"default_statistics_target", statisticsTarget},
		tuningItem{"log_checkpoints", "on"},
		tuningItem{"log_lock_waits", "on"},
		tuningItem{"log_min_duration_statement", "100"},
		tuningItem{"track_io_timing", "on"},
		tuningItem{"log_autovacuum_min_duration", "1s"},
		tuningItem{"autovacuum_max_workers", autovacuumWorkers},
		tuningItem{"autovacuum_naptime", "1min"},
		tuningItem{"autovacuum_vacuum_threshold", "500"},
		tuningItem{"autovacuum_analyze_threshold", "250"},
	)
	if oltp {
		items = append(items,
			tuningItem{"autovacuum_vacuum_scale_factor", "0.08"},
			tuningItem{"autovacuum_analyze_scale_factor", "0.04"},
		)
	}
	items = append(items,
		tuningItem{"autovacuum_freeze_max_age", "1000000000"},
		tuningItem{"deadlock_timeout", "50ms"},
		tuningItem{"idle_in_transaction_session_timeout", "10min"},
	)
	return items
}

func redisTuning(tpl string, basis tuningBasis, version string) []tuningItem {
	tpl = normalizeTuningTemplate(tpl)
	if tpl == TuneTemplateNone {
		return nil
	}
	oltp := tpl == TuneTemplateOLTP
	ratio := 0.25
	if oltp {
		ratio = 0.5
	}
	if basis.MemLimited {
		ratio = 0.75
	}
	maxMemory := maxInt(int(float64(basis.MemMB)*ratio), 64)

	items := []tuningItem{
		{"maxmemory", fmt.Sprintf("%dmb", maxMemory)},
		{"maxmemory-policy", "allkeys-lru"},
		{"save", "3600 1,300 100,60 10000"},
	}
	if !oltp {
		return append(items,
			tuningItem{"maxclients", "2000"},
			tuningItem{"tcp-backlog", "511"},
			tuningItem{"appendonly", "no"},
		)
	}
	items = append(items,
		tuningItem{"maxclients", "6000"},
		tuningItem{"tcp-backlog", "1024"},
		tuningItem{"appendonly", "yes"},
		tuningItem{"appendfsync", "everysec"},
		tuningItem{"lazyfree-lazy-eviction", "yes"},
		tuningItem{"lazyfree-lazy-expire", "yes"},
		tuningItem{"lazyfree-lazy-server-del", "yes"},
		tuningItem{"replica-lazy-flush", "yes"},
	)
	if appMajorVersion(version) >= 6 {
		items = append(items, tuningItem{"lazyfree-lazy-user-del", "yes"})
		if basis.CPU >= 4 {
			items = append(items, tuningItem{"io-threads", strconv.Itoa(minInt(basis.CPU/2, 8))})
		}
	}
	return items
}

func pgBouncerPoolSize(tpl string) (defaultPool, reservePool, maxDBConn, maxClientConn int) {
	if normalizeTuningTemplate(tpl) == TuneTemplateOLTP {
		return 50, 30, 400, 20000
	}
	return 20, 5, 100, 2000
}

func appTuningPreview(key, version, tpl string, basis tuningBasis, pgbouncer bool) []tuningItem {
	switch key {
	case "postgresql":
		return pgTuning(tpl, basis, version, pgbouncer)
	case "redis":
		return redisTuning(tpl, basis, version)
	}
	return nil
}

func (a AppService) LoadTuningPreview(req request.AppTuningPreview) (*response.AppTuningPreview, error) {
	basis := loadTuningBasis(req.AppContainerConfig)
	res := &response.AppTuningPreview{
		Recommend: recommendTuningTemplate(basis.CPU),
		CPU:       basis.CPU,
		MemMB:     basis.MemMB,
		DiskGB:    basis.DiskGB,
	}
	tpl := req.Template
	if tpl == "" {
		tpl = res.Recommend
	}
	res.Template = normalizeTuningTemplate(tpl)
	for _, item := range appTuningPreview(req.Key, req.Version, res.Template, basis, req.Key == "postgresql" && req.PgBouncer) {
		res.Items = append(res.Items, response.AppTuningItem{Key: item.Key, Value: item.Value})
	}
	return res, nil
}
