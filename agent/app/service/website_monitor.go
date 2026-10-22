package service

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"math/bits"
	"os"
	"path"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"gorm.io/gorm"
)

// Website monitoring is collected by the guard Lua (guard/monitor.lua): each
// worker writes one aggregated line per site and minute to monitor.log, and
// blocked or monitored requests go to events.log. This file merges those
// lines into website_monitor.db.

// Upper bounds (ms) of the response time histogram, mirrored from the Lua.
var monitorRtBounds = []float64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

const (
	monitorTopKeep    = 200
	monitorCommitLag  = 2 * 60
	monitorRotateSize = 64 * 1024 * 1024
	monitorRotateAge  = time.Hour
	hllPrecision      = 14
)

// ---- HyperLogLog (dense, 2^14 registers) used for day-level UV/IP ----

type hll []byte

func newHLL() hll { return make(hll, 1<<hllPrecision) }

func loadHLL(data []byte) hll {
	if len(data) != 1<<hllPrecision {
		return newHLL()
	}
	return hll(append([]byte(nil), data...))
}

func mix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func (h hll) add(v uint64) {
	x := mix64(v)
	idx := x >> (64 - hllPrecision)
	rank := uint8(bits.LeadingZeros64(x<<hllPrecision|1<<(hllPrecision-1)) + 1)
	if rank > h[idx] {
		h[idx] = rank
	}
}

func (h hll) count() int64 {
	m := float64(len(h))
	sum, zeros := 0.0, 0.0
	for _, r := range h {
		sum += math.Pow(2, -float64(r))
		if r == 0 {
			zeros++
		}
	}
	estimate := 0.7213 / (1 + 1.079/m) * m * m / sum
	if estimate <= 2.5*m && zeros > 0 {
		estimate = m * math.Log(m/zeros)
	}
	return int64(estimate + 0.5)
}

func (h hll) merge(o hll) {
	for i := range h {
		if i < len(o) && o[i] > h[i] {
			h[i] = o[i]
		}
	}
}

// ---- tolerant decoding of the Lua JSON (empty tables may be {} or []) ----

func decodeCountMap(raw json.RawMessage) map[string]int64 {
	out := make(map[string]int64)
	if len(raw) == 0 || raw[0] != '{' {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func decodeHashes(raw json.RawMessage) []uint64 {
	var out []uint64
	if len(raw) == 0 || raw[0] != '[' {
		return out
	}
	var floats []float64
	if json.Unmarshal(raw, &floats) == nil {
		for _, f := range floats {
			out = append(out, uint64(f))
		}
	}
	return out
}

type monitorLine struct {
	Site    string                     `json:"site"`
	Minute  int64                      `json:"minute"`
	Req     int64                      `json:"req"`
	In      float64                    `json:"in"`
	Out     float64                    `json:"out"`
	S2      int64                      `json:"s2"`
	S3      int64                      `json:"s3"`
	S4      int64                      `json:"s4"`
	S5      int64                      `json:"s5"`
	Blocked int64                      `json:"blocked"`
	RtSum   float64                    `json:"rtSum"`
	Hist    []int64                    `json:"hist"`
	UV      json.RawMessage            `json:"uv"`
	IPs     json.RawMessage            `json:"ips"`
	Top     map[string]json.RawMessage `json:"top"`
	Slow    json.RawMessage            `json:"slow"`
}

type monitorSlowLine struct {
	T  int64   `json:"t"`
	M  string  `json:"m"`
	U  string  `json:"u"`
	IP string  `json:"ip"`
	S  int     `json:"s"`
	Rt float64 `json:"rt"`
}

var monitorTopDims = []string{"url", "ip", "ref", "ua", "country", "err"}

type monitorAgg struct {
	site    string
	minute  int64
	req     int64
	in      int64
	out     int64
	s2      int64
	s3      int64
	s4      int64
	s5      int64
	blocked int64
	rtSum   float64
	hist    []int64
	uv      map[uint64]struct{}
	ips     map[uint64]struct{}
	top     map[string]map[string]int64
}

func newMonitorAgg(site string, minute int64) *monitorAgg {
	agg := &monitorAgg{
		site: site, minute: minute,
		hist: make([]int64, len(monitorRtBounds)+1),
		uv:   make(map[uint64]struct{}),
		ips:  make(map[uint64]struct{}),
		top:  make(map[string]map[string]int64),
	}
	for _, dim := range monitorTopDims {
		agg.top[dim] = make(map[string]int64)
	}
	return agg
}

func (a *monitorAgg) add(line monitorLine) {
	a.req += line.Req
	a.in += int64(line.In)
	a.out += int64(line.Out)
	a.s2 += line.S2
	a.s3 += line.S3
	a.s4 += line.S4
	a.s5 += line.S5
	a.blocked += line.Blocked
	a.rtSum += line.RtSum
	for i, v := range line.Hist {
		if i < len(a.hist) {
			a.hist[i] += v
		}
	}
	for _, h := range decodeHashes(line.UV) {
		a.uv[h] = struct{}{}
	}
	for _, h := range decodeHashes(line.IPs) {
		a.ips[h] = struct{}{}
	}
	for _, dim := range monitorTopDims {
		for k, v := range decodeCountMap(line.Top[dim]) {
			a.top[dim][k] += v
		}
	}
}

func trimTop(m map[string]int64, keep int) map[string]int64 {
	if len(m) <= keep {
		return m
	}
	type kv struct {
		k string
		v int64
	}
	list := make([]kv, 0, len(m))
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	out := make(map[string]int64, keep)
	for _, item := range list[:keep] {
		out[item.k] = item.v
	}
	return out
}

func encodeTop(m map[string]int64) string {
	data, _ := json.Marshal(trimTop(m, monitorTopKeep))
	return string(data)
}

func decodeTop(s string) map[string]int64 {
	out := make(map[string]int64)
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func mergeTopString(old string, add map[string]int64) string {
	merged := decodeTop(old)
	for k, v := range add {
		merged[k] += v
	}
	return encodeTop(merged)
}

func decodeHist(s string) []int64 {
	hist := make([]int64, len(monitorRtBounds)+1)
	var stored []int64
	if s != "" && json.Unmarshal([]byte(s), &stored) == nil {
		for i, v := range stored {
			if i < len(hist) {
				hist[i] = v
			}
		}
	}
	return hist
}

func encodeHist(h []int64) string {
	data, _ := json.Marshal(h)
	return string(data)
}

// ---- collector ----

type monitorFileState struct {
	Inode      uint64    `json:"inode"`
	Offset     int64     `json:"offset"`
	RotateTime time.Time `json:"rotateTime"`
}

type websiteMonitorCollector struct {
	mu      sync.Mutex
	pending map[string]*monitorAgg
	lastGC  time.Time
}

var monitorCollector = &websiteMonitorCollector{pending: make(map[string]*monitorAgg)}

// CollectWebsiteMonitor is run every minute by the agent cron.
func CollectWebsiteMonitor() {
	monitorCollector.collect()
}

func fileInode(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(stat.Ino)
	}
	return 0
}

func loadMonitorStates(file string) map[string]*monitorFileState {
	states := make(map[string]*monitorFileState)
	if content, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(content, &states)
	}
	return states
}

// readNewLines returns the complete lines appended since the stored offset
// and rotates the file once it is large or old enough.
func readNewLines(file string, state *monitorFileState) []string {
	info, err := os.Stat(file)
	if err != nil {
		return nil
	}
	if inode := fileInode(info); inode != state.Inode || info.Size() < state.Offset {
		state.Inode = inode
		state.Offset = 0
	}
	if state.RotateTime.IsZero() {
		state.RotateTime = time.Now()
	}
	rotate := info.Size() >= monitorRotateSize || time.Since(state.RotateTime) >= monitorRotateAge
	readFile := file
	if rotate {
		rotated := file + ".1"
		if err = os.Rename(file, rotated); err != nil {
			return nil
		}
		// Let workers that already opened the old file finish their write.
		time.Sleep(2 * time.Second)
		readFile = rotated
		defer os.Remove(rotated)
	}
	fd, err := os.Open(readFile)
	if err != nil {
		return nil
	}
	defer fd.Close()
	if _, err = fd.Seek(state.Offset, io.SeekStart); err != nil {
		return nil
	}
	var lines []string
	reader := bufio.NewReaderSize(fd, 1<<20)
	offset := state.Offset
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// A partial last line is picked up on the next run, except when
			// the file was rotated away.
			if rotate && len(line) > 0 {
				lines = append(lines, line)
			}
			break
		}
		offset += int64(len(line))
		lines = append(lines, line[:len(line)-1])
	}
	if rotate {
		state.Offset = 0
		state.Inode = 0
		state.RotateTime = time.Now()
	} else {
		state.Offset = offset
	}
	return lines
}

func (c *websiteMonitorCollector) collect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if global.WebsiteMonitorDB == nil {
		return
	}
	install, err := getAppInstallByKey(constant.AppOpenresty)
	if err != nil {
		return
	}
	logDir := guardLogDir(install)
	if _, err = os.Stat(logDir); err != nil {
		return
	}
	stateFile := path.Join(logDir, ".collector.json")
	states := loadMonitorStates(stateFile)
	for _, name := range []string{"monitor.log", "events.log"} {
		if states[name] == nil {
			states[name] = &monitorFileState{}
		}
	}

	for _, line := range readNewLines(path.Join(logDir, "monitor.log"), states["monitor.log"]) {
		var item monitorLine
		if json.Unmarshal([]byte(line), &item) != nil || item.Site == "" {
			continue
		}
		key := item.Site + "|" + time.Unix(item.Minute, 0).Format("200601021504")
		agg := c.pending[key]
		if agg == nil {
			agg = newMonitorAgg(item.Site, item.Minute)
			c.pending[key] = agg
		}
		agg.add(item)
		c.saveSlow(item)
	}
	c.saveEvents(readNewLines(path.Join(logDir, "events.log"), states["events.log"]))

	cutoff := time.Now().Unix() - monitorCommitLag
	for key, agg := range c.pending {
		if agg.minute <= cutoff {
			if err := commitMonitorAgg(agg); err != nil {
				global.LOG.Errorf("save website monitor of %s failed: %v", agg.site, err)
			}
			delete(c.pending, key)
		}
	}
	if content, err := json.Marshal(states); err == nil {
		_ = os.WriteFile(stateFile, content, constant.FilePerm)
	}
	if time.Since(c.lastGC) >= time.Hour {
		c.lastGC = time.Now()
		cleanWebsiteMonitor()
		// Re-apply the guard in case a site config or nginx.conf was
		// rewritten outside the guard (OpenResty upgrade, manual edit).
		syncGuardQuietly()
	}
}

func (c *websiteMonitorCollector) saveSlow(item monitorLine) {
	if len(item.Slow) == 0 || item.Slow[0] != '[' {
		return
	}
	var slow []monitorSlowLine
	if json.Unmarshal(item.Slow, &slow) != nil || len(slow) == 0 {
		return
	}
	rows := make([]model.WebsiteMonitorSlow, 0, len(slow))
	for _, s := range slow {
		rows = append(rows, model.WebsiteMonitorSlow{Site: item.Site, Time: s.T, Method: s.M, URI: s.U, IP: s.IP, Status: s.S, RequestTime: s.Rt})
	}
	_ = global.WebsiteMonitorDB.CreateInBatches(rows, 100).Error
}

func (c *websiteMonitorCollector) saveEvents(lines []string) {
	rows := make([]model.WebsiteGuardEvent, 0, len(lines))
	for _, line := range lines {
		var event model.WebsiteGuardEvent
		if json.Unmarshal([]byte(line), &event) != nil || event.Site == "" {
			continue
		}
		event.ID = 0
		rows = append(rows, event)
	}
	if len(rows) > 0 {
		_ = global.WebsiteMonitorDB.CreateInBatches(rows, 200).Error
	}
}

func hashesToUint64(set map[uint64]struct{}) []uint64 {
	out := make([]uint64, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	return out
}

func dayStart(ts int64) int64 {
	t := time.Unix(ts, 0)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Unix()
}

func commitMonitorAgg(agg *monitorAgg) error {
	return global.WebsiteMonitorDB.Transaction(func(tx *gorm.DB) error {
		var minute model.WebsiteMonitorMinute
		tx.Where("site = ? AND minute = ?", agg.site, agg.minute).First(&minute)
		hist := decodeHist(minute.RtHist)
		for i, v := range agg.hist {
			hist[i] += v
		}
		minute.Site, minute.Minute = agg.site, agg.minute
		minute.Requests += agg.req
		minute.BytesIn += agg.in
		minute.BytesOut += agg.out
		// A late line for an already saved minute can only be added, not
		// de-duplicated; such lines are rare thanks to the commit lag.
		minute.UV += int64(len(agg.uv))
		minute.IP += int64(len(agg.ips))
		minute.S2xx += agg.s2
		minute.S3xx += agg.s3
		minute.S4xx += agg.s4
		minute.S5xx += agg.s5
		minute.Blocked += agg.blocked
		minute.RtSum += agg.rtSum
		minute.RtHist = encodeHist(hist)
		minute.TopURL = mergeTopString(minute.TopURL, agg.top["url"])
		minute.TopIP = mergeTopString(minute.TopIP, agg.top["ip"])
		minute.TopReferer = mergeTopString(minute.TopReferer, agg.top["ref"])
		minute.TopUA = mergeTopString(minute.TopUA, agg.top["ua"])
		minute.TopCountry = mergeTopString(minute.TopCountry, agg.top["country"])
		minute.TopErrURL = mergeTopString(minute.TopErrURL, agg.top["err"])
		if err := tx.Save(&minute).Error; err != nil {
			return err
		}

		var day model.WebsiteMonitorDay
		start := dayStart(agg.minute)
		tx.Where("site = ? AND day = ?", agg.site, start).First(&day)
		dayHist := decodeHist(day.RtHist)
		for i, v := range agg.hist {
			dayHist[i] += v
		}
		uvSketch, ipSketch := loadHLL(day.UVSketch), loadHLL(day.IPSketch)
		for _, h := range hashesToUint64(agg.uv) {
			uvSketch.add(h)
		}
		for _, h := range hashesToUint64(agg.ips) {
			ipSketch.add(h)
		}
		day.Site, day.Day = agg.site, start
		day.Requests += agg.req
		day.BytesIn += agg.in
		day.BytesOut += agg.out
		day.UV = uvSketch.count()
		day.IP = ipSketch.count()
		day.S2xx += agg.s2
		day.S3xx += agg.s3
		day.S4xx += agg.s4
		day.S5xx += agg.s5
		day.Blocked += agg.blocked
		day.RtSum += agg.rtSum
		day.RtHist = encodeHist(dayHist)
		day.UVSketch, day.IPSketch = uvSketch, ipSketch
		day.TopURL = mergeTopString(day.TopURL, agg.top["url"])
		day.TopIP = mergeTopString(day.TopIP, agg.top["ip"])
		day.TopReferer = mergeTopString(day.TopReferer, agg.top["ref"])
		day.TopUA = mergeTopString(day.TopUA, agg.top["ua"])
		day.TopCountry = mergeTopString(day.TopCountry, agg.top["country"])
		day.TopErrURL = mergeTopString(day.TopErrURL, agg.top["err"])
		return tx.Save(&day).Error
	})
}

func cleanWebsiteMonitor() {
	cfg := loadGuardGlobalConfig()
	now := time.Now()
	db := global.WebsiteMonitorDB
	db.Where("minute < ?", now.AddDate(0, 0, -cfg.MinuteDays).Unix()).Delete(&model.WebsiteMonitorMinute{})
	db.Where("time < ?", now.AddDate(0, 0, -cfg.MinuteDays).Unix()).Delete(&model.WebsiteMonitorSlow{})
	db.Where("day < ?", now.AddDate(0, 0, -cfg.DayDays).Unix()).Delete(&model.WebsiteMonitorDay{})
	db.Where("time < ?", now.AddDate(0, 0, -cfg.EventDays).Unix()).Delete(&model.WebsiteGuardEvent{})
}

func deleteWebsiteMonitorData(alias string) {
	if global.WebsiteMonitorDB == nil {
		return
	}
	db := global.WebsiteMonitorDB
	db.Where("site = ?", alias).Delete(&model.WebsiteMonitorMinute{})
	db.Where("site = ?", alias).Delete(&model.WebsiteMonitorDay{})
	db.Where("site = ?", alias).Delete(&model.WebsiteMonitorSlow{})
	db.Where("site = ?", alias).Delete(&model.WebsiteGuardEvent{})
}

// ---- queries ----

func histPercentile(hist []int64, p float64) float64 {
	var total int64
	for _, v := range hist {
		total += v
	}
	if total == 0 {
		return 0
	}
	target := float64(total) * p
	var cum float64
	for i, v := range hist {
		if cum+float64(v) >= target {
			if i >= len(monitorRtBounds) {
				return monitorRtBounds[len(monitorRtBounds)-1]
			}
			lo := 0.0
			if i > 0 {
				lo = monitorRtBounds[i-1]
			}
			hi := monitorRtBounds[i]
			if v == 0 {
				return hi
			}
			return lo + (hi-lo)*(target-cum)/float64(v)
		}
		cum += float64(v)
	}
	return monitorRtBounds[len(monitorRtBounds)-1]
}

// monitorRow is the common view of minute and day rows.
type monitorRow struct {
	Time       int64
	Requests   int64
	BytesIn    int64
	BytesOut   int64
	UV         int64
	IP         int64
	S2xx       int64
	S3xx       int64
	S4xx       int64
	S5xx       int64
	Blocked    int64
	RtSum      float64
	RtHist     string
	TopURL     string
	TopIP      string
	TopReferer string
	TopUA      string
	TopCountry string
	TopErrURL  string
	UVSketch   []byte
	IPSketch   []byte
}

// loadMonitorRows uses minute rows while the range is inside their
// retention, day rows otherwise.
func loadMonitorRows(req dto.WebsiteMonitorReq) ([]monitorRow, bool) {
	cfg := loadGuardGlobalConfig()
	useMinute := req.StartTime >= time.Now().AddDate(0, 0, -cfg.MinuteDays).Unix()
	var rows []monitorRow
	db := global.WebsiteMonitorDB
	if useMinute {
		var list []model.WebsiteMonitorMinute
		query := db.Where("minute >= ? AND minute <= ?", req.StartTime, req.EndTime)
		if req.Site != "" {
			query = query.Where("site = ?", req.Site)
		}
		query.Order("minute").Find(&list)
		for _, m := range list {
			rows = append(rows, monitorRow{m.Minute, m.Requests, m.BytesIn, m.BytesOut, m.UV, m.IP, m.S2xx, m.S3xx, m.S4xx, m.S5xx, m.Blocked, m.RtSum, m.RtHist, m.TopURL, m.TopIP, m.TopReferer, m.TopUA, m.TopCountry, m.TopErrURL, nil, nil})
		}
		return rows, true
	}
	var list []model.WebsiteMonitorDay
	query := db.Where("day >= ? AND day <= ?", dayStart(req.StartTime), req.EndTime)
	if req.Site != "" {
		query = query.Where("site = ?", req.Site)
	}
	query.Order("day").Find(&list)
	for _, d := range list {
		rows = append(rows, monitorRow{d.Day, d.Requests, d.BytesIn, d.BytesOut, d.UV, d.IP, d.S2xx, d.S3xx, d.S4xx, d.S5xx, d.Blocked, d.RtSum, d.RtHist, d.TopURL, d.TopIP, d.TopReferer, d.TopUA, d.TopCountry, d.TopErrURL, d.UVSketch, d.IPSketch})
	}
	return rows, false
}

func (s *WebsiteGuardService) LoadMonitorOverview(req dto.WebsiteMonitorReq) (*dto.WebsiteMonitorOverview, error) {
	res := &dto.WebsiteMonitorOverview{}
	rows, minuteRows := loadMonitorRows(req)
	hist := make([]int64, len(monitorRtBounds)+1)
	var rtSum float64
	perMinute := make(map[int64]int64)
	for _, r := range rows {
		res.Requests += r.Requests
		res.BytesIn += r.BytesIn
		res.BytesOut += r.BytesOut
		res.S2xx += r.S2xx
		res.S3xx += r.S3xx
		res.S4xx += r.S4xx
		res.S5xx += r.S5xx
		res.Blocked += r.Blocked
		rtSum += r.RtSum
		for i, v := range decodeHist(r.RtHist) {
			hist[i] += v
		}
		if minuteRows {
			perMinute[r.Time] += r.Requests
		}
	}
	for _, v := range perMinute {
		if qps := float64(v) / 60; qps > res.PeakQPS {
			res.PeakQPS = qps
		}
	}
	if res.Requests > 0 {
		res.AvgRt = rtSum / float64(res.Requests)
	}
	res.P95Rt = histPercentile(hist, 0.95)

	// Distinct visitors come from the day sketches of the covered days.
	var days []model.WebsiteMonitorDay
	query := global.WebsiteMonitorDB.Where("day >= ? AND day <= ?", dayStart(req.StartTime), req.EndTime)
	if req.Site != "" {
		query = query.Where("site = ?", req.Site)
	}
	query.Find(&days)
	uv, ip := newHLL(), newHLL()
	for _, d := range days {
		uv.merge(loadHLL(d.UVSketch))
		ip.merge(loadHLL(d.IPSketch))
	}
	if len(days) > 0 {
		res.UV, res.IP = uv.count(), ip.count()
	}
	return res, nil
}

func monitorTrendInterval(start, end int64, minuteRows bool) int64 {
	if !minuteRows {
		return 86400
	}
	switch span := end - start; {
	case span <= 6*3600:
		return 60
	case span <= 2*86400:
		return 300
	default:
		return 3600
	}
}

func (s *WebsiteGuardService) LoadMonitorTrend(req dto.WebsiteMonitorReq) (*dto.WebsiteMonitorTrend, error) {
	rows, minuteRows := loadMonitorRows(req)
	interval := monitorTrendInterval(req.StartTime, req.EndTime, minuteRows)
	type bucket struct {
		point dto.WebsiteMonitorPoint
		rtSum float64
		hist  []int64
	}
	buckets := make(map[int64]*bucket)
	for _, r := range rows {
		key := r.Time - r.Time%interval
		if interval == 86400 {
			key = dayStart(r.Time)
		}
		b := buckets[key]
		if b == nil {
			b = &bucket{point: dto.WebsiteMonitorPoint{Time: key}, hist: make([]int64, len(monitorRtBounds)+1)}
			buckets[key] = b
		}
		p := &b.point
		p.Requests += r.Requests
		p.UV += r.UV
		p.IP += r.IP
		p.BytesIn += r.BytesIn
		p.BytesOut += r.BytesOut
		p.S2xx += r.S2xx
		p.S3xx += r.S3xx
		p.S4xx += r.S4xx
		p.S5xx += r.S5xx
		p.Blocked += r.Blocked
		b.rtSum += r.RtSum
		for i, v := range decodeHist(r.RtHist) {
			b.hist[i] += v
		}
	}
	res := &dto.WebsiteMonitorTrend{Interval: interval, Points: make([]dto.WebsiteMonitorPoint, 0, len(buckets))}
	for _, b := range buckets {
		p := b.point
		p.QPS = float64(p.Requests) / float64(interval)
		if p.Requests > 0 {
			p.AvgRt = b.rtSum / float64(p.Requests)
		}
		p.P95Rt = histPercentile(b.hist, 0.95)
		res.Points = append(res.Points, p)
	}
	sort.Slice(res.Points, func(i, j int) bool { return res.Points[i].Time < res.Points[j].Time })
	return res, nil
}

func topList(m map[string]int64, n int) []dto.GuardCount {
	list := make([]dto.GuardCount, 0, len(m))
	for k, v := range m {
		list = append(list, dto.GuardCount{Name: k, Count: v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Count > list[j].Count })
	if len(list) > n {
		list = list[:n]
	}
	return list
}

func (s *WebsiteGuardService) LoadMonitorTop(req dto.WebsiteMonitorReq) (*dto.WebsiteMonitorTop, error) {
	rows, _ := loadMonitorRows(req)
	merged := make(map[string]map[string]int64)
	for _, dim := range monitorTopDims {
		merged[dim] = make(map[string]int64)
	}
	for _, r := range rows {
		for dim, raw := range map[string]string{"url": r.TopURL, "ip": r.TopIP, "ref": r.TopReferer, "ua": r.TopUA, "country": r.TopCountry, "err": r.TopErrURL} {
			for k, v := range decodeTop(raw) {
				merged[dim][k] += v
			}
		}
	}
	return &dto.WebsiteMonitorTop{
		URL:     topList(merged["url"], 20),
		IP:      topList(merged["ip"], 20),
		Referer: topList(merged["ref"], 20),
		UA:      topList(merged["ua"], 20),
		Country: topList(merged["country"], 20),
		ErrURL:  topList(merged["err"], 20),
	}, nil
}

func (s *WebsiteGuardService) SearchMonitorSlow(req dto.GuardEventSearch) (int64, []model.WebsiteMonitorSlow, error) {
	var (
		total int64
		list  []model.WebsiteMonitorSlow
	)
	query := global.WebsiteMonitorDB.Model(&model.WebsiteMonitorSlow{})
	if req.Site != "" {
		query = query.Where("site = ?", req.Site)
	}
	if req.StartTime > 0 {
		query = query.Where("time >= ?", req.StartTime)
	}
	if req.EndTime > 0 {
		query = query.Where("time <= ?", req.EndTime)
	}
	if err := query.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	err := query.Order("request_time desc").Limit(req.PageSize).Offset((req.Page - 1) * req.PageSize).Find(&list).Error
	return total, list, err
}

func (s *WebsiteGuardService) SearchEvents(req dto.GuardEventSearch) (int64, []model.WebsiteGuardEvent, error) {
	var (
		total int64
		list  []model.WebsiteGuardEvent
	)
	query := global.WebsiteMonitorDB.Model(&model.WebsiteGuardEvent{})
	if req.Site != "" {
		query = query.Where("site = ?", req.Site)
	}
	if req.Type != "" {
		query = query.Where("type = ?", req.Type)
	}
	if req.IP != "" {
		query = query.Where("ip = ?", req.IP)
	}
	if req.StartTime > 0 {
		query = query.Where("time >= ?", req.StartTime)
	}
	if req.EndTime > 0 {
		query = query.Where("time <= ?", req.EndTime)
	}
	if err := query.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	err := query.Order("time desc, id desc").Limit(req.PageSize).Offset((req.Page - 1) * req.PageSize).Find(&list).Error
	return total, list, err
}

func (s *WebsiteGuardService) LoadEventStats(req dto.WebsiteMonitorReq) (*dto.GuardEventStats, error) {
	res := &dto.GuardEventStats{ByType: make(map[string]int64), BySite: make(map[string]int64)}
	base := func() *gorm.DB {
		query := global.WebsiteMonitorDB.Model(&model.WebsiteGuardEvent{}).Where("time >= ? AND time <= ?", req.StartTime, req.EndTime)
		if req.Site != "" {
			query = query.Where("site = ?", req.Site)
		}
		return query
	}
	if err := base().Count(&res.Total).Error; err != nil {
		return nil, err
	}
	var grouped []struct {
		Name  string
		Count int64
	}
	base().Select("type as name, count(*) as count").Group("type").Scan(&grouped)
	for _, g := range grouped {
		res.ByType[g.Name] = g.Count
	}
	grouped = nil
	base().Select("site as name, count(*) as count").Group("site").Scan(&grouped)
	for _, g := range grouped {
		res.BySite[g.Name] = g.Count
	}
	base().Select("ip as name, count(*) as count").Group("ip").Order("count desc").Limit(10).Scan(&res.TopIP)
	bucket := int64(3600)
	if req.EndTime-req.StartTime > 2*86400 {
		bucket = 86400
	}
	// Group in SQL; daily buckets are aligned to local midnight.
	_, offset := time.Now().Zone()
	var trend []struct {
		Bucket int64
		Count  int64
	}
	base().Select("(time + ?) - ((time + ?) % ?) - ? as bucket, count(*) as count", offset, offset, bucket, offset).
		Group("bucket").Order("bucket").Scan(&trend)
	for _, t := range trend {
		res.Trend = append(res.Trend, dto.GuardCount{Name: time.Unix(t.Bucket, 0).Format(time.RFC3339), Count: t.Count})
	}
	return res, nil
}
