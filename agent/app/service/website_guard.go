package service

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	guardLua "github.com/1Panel-dev/1Panel/agent/cmd/server/guard"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/geo"
	"github.com/1Panel-dev/1Panel/agent/utils/nginx"
	"github.com/1Panel-dev/1Panel/agent/utils/nginx/components"
	"github.com/1Panel-dev/1Panel/agent/utils/nginx/parser"
)

// The guard puts SafeLine's detection engine (a trimmed SafeLine CE stack,
// see website_guard_safeline.go, reached through the vendored lua-resty-t1k
// client) plus our own Lua for IP lists, region blocking, CC limiting and
// traffic monitoring in front of the OpenResty sites.
//
// The http-level init_worker/access/log phases and lua_package_path are
// already taken by the 1pwaf include shipped with every OpenResty install,
// so the guard hooks in per server block: "set $guard_site", plus
// access_by_lua_file/log_by_lua_file pointing at the entry scripts. A server
// level handler overrides the inherited 1pwaf one, which is why enabling the
// guard is refused while 1pwaf itself is switched on.

const (
	guardSettingKey = "GuardConfig"

	guardHTTPConfName = "1panel-guard.conf"
	guardGeoConfName  = "1panel-guard-geo.conf"

	guardContainerRoot = "/www/guard"
	guardAccessEntry   = guardContainerRoot + "/lua/guard/access_entry.lua"
	guardLogEntry      = guardContainerRoot + "/lua/guard/log_entry.lua"
	guardSiteVar       = "$guard_site"
	guardContainerLog  = "/var/log/nginx/guard"
)

var guardSyncMu sync.Mutex

type WebsiteGuardService struct{}

type IWebsiteGuardService interface {
	GetStatus() (*dto.GuardStatus, error)
	UpdateGlobal(req dto.GuardGlobalConfig) error
	OperateDetector(req dto.GuardDetectorOperate) (string, error)
	GetSite(websiteID uint) (*dto.GuardSiteConfig, error)
	UpdateSite(req dto.GuardSiteConfig) error
	SearchEvents(req dto.GuardEventSearch) (int64, []model.WebsiteGuardEvent, error)
	LoadEventStats(req dto.WebsiteMonitorReq) (*dto.GuardEventStats, error)
	LoadCountries() ([]dto.GuardCountry, error)

	LoadMonitorOverview(req dto.WebsiteMonitorReq) (*dto.WebsiteMonitorOverview, error)
	LoadMonitorTrend(req dto.WebsiteMonitorReq) (*dto.WebsiteMonitorTrend, error)
	LoadMonitorTop(req dto.WebsiteMonitorReq) (*dto.WebsiteMonitorTop, error)
	SearchMonitorSlow(req dto.GuardEventSearch) (int64, []model.WebsiteMonitorSlow, error)
}

func NewIWebsiteGuardService() IWebsiteGuardService {
	return &WebsiteGuardService{}
}

func defaultGuardGlobalConfig() dto.GuardGlobalConfig {
	return dto.GuardGlobalConfig{
		GeoMode:       "none",
		SlowThreshold: 1000,
		MinuteDays:    7,
		DayDays:       90,
		EventDays:     30,
		Mode:          "block",
		CCEnable:      true,
		CCDuration:    60,
		CCThreshold:   300,
		CCBanTime:     600,
	}
}

func loadGuardGlobalConfig() dto.GuardGlobalConfig {
	cfg := defaultGuardGlobalConfig()
	value, _ := settingRepo.GetValueByKey(guardSettingKey)
	if strings.TrimSpace(value) != "" {
		_ = json.Unmarshal([]byte(value), &cfg)
	}
	if cfg.GeoMode == "" {
		cfg.GeoMode = "none"
	}
	if cfg.MinuteDays <= 0 {
		cfg.MinuteDays = 7
	}
	if cfg.DayDays <= 0 {
		cfg.DayDays = 90
	}
	if cfg.EventDays <= 0 {
		cfg.EventDays = 30
	}
	return cfg
}

func guardRootDir() string {
	return path.Join(GetWebSiteRootDir(), "guard")
}

func guardLogDir(install model.AppInstall) string {
	return path.Join(install.GetPath(), "log", "guard")
}

// guardWafActive reports whether the bundled 1pwaf engine is switched on.
func guardWafActive(install model.AppInstall) bool {
	content, err := os.ReadFile(path.Join(install.GetPath(), "1pwaf", "data", "conf", "global.json"))
	if err != nil {
		return false
	}
	var cfg struct {
		Waf struct {
			State string `json:"state"`
		} `json:"waf"`
	}
	if err = json.Unmarshal(content, &cfg); err != nil {
		return false
	}
	return cfg.Waf.State == "on"
}

func guardContainerState(name string) (bool, string) {
	out, err := cmd.NewCommandMgr(cmd.WithTimeout(10*time.Second)).RunWithStdout("docker", "inspect", "--format", "{{.State.Status}}", name)
	if err != nil {
		return false, ""
	}
	return true, strings.TrimSpace(out)
}

func splitGuardList(value string) []string {
	items := make([]string, 0)
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			items = append(items, line)
		}
	}
	return items
}

func normalizeGuardIPs(list []string) ([]string, error) {
	items := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			if _, _, err := net.ParseCIDR(item); err != nil {
				return nil, buserr.WithName("ErrGuardInvalidIP", item)
			}
		} else if net.ParseIP(item) == nil {
			return nil, buserr.WithName("ErrGuardInvalidIP", item)
		}
		items = append(items, item)
	}
	return items, nil
}

func normalizeGuardCountries(list []string) []string {
	items := make([]string, 0, len(list))
	seen := make(map[string]bool)
	for _, item := range list {
		item = strings.ToUpper(strings.TrimSpace(item))
		if len(item) == 2 && !seen[item] {
			seen[item] = true
			items = append(items, item)
		}
	}
	sort.Strings(items)
	return items
}

func guardSiteToDTO(site model.WebsiteGuard) *dto.GuardSiteConfig {
	countries := make([]string, 0)
	for _, c := range strings.Split(site.Countries, ",") {
		if c = strings.TrimSpace(c); c != "" {
			countries = append(countries, c)
		}
	}
	return &dto.GuardSiteConfig{
		WebsiteID:   site.WebsiteID,
		Enable:      site.Enable,
		Mode:        site.Mode,
		CCEnable:    site.CCEnable,
		CCDuration:  site.CCDuration,
		CCThreshold: site.CCThreshold,
		CCBanTime:   site.CCBanTime,
		IPAllow:     splitGuardList(site.IPAllow),
		IPDeny:      splitGuardList(site.IPDeny),
		GeoMode:     site.GeoMode,
		Countries:   countries,
	}
}

func loadGuardSite(websiteID uint) (model.WebsiteGuard, bool) {
	var site model.WebsiteGuard
	if err := global.DB.Where("website_id = ?", websiteID).First(&site).Error; err != nil {
		return site, false
	}
	return site, true
}

func (s *WebsiteGuardService) GetStatus() (*dto.GuardStatus, error) {
	res := &dto.GuardStatus{
		Config:          loadGuardGlobalConfig(),
		SafelineVersion: safelineVersion,
		Sites:           make([]dto.GuardSiteBrief, 0),
	}
	if install, err := getAppInstallByKey(constant.AppOpenresty); err == nil {
		res.OpenrestyInstalled = true
		res.OpenrestyRunning = install.Status == constant.StatusRunning
		res.WafConflict = guardWafActive(install)
	}
	res.DetectorInstalled, res.DetectorStatus = guardContainerState(safelineDetectorContainer)
	_, res.MgtStatus = guardContainerState(safelineMgtContainer)
	if install, err := getAppInstallByKey(constant.AppOpenresty); err == nil {
		res.ConsolePort = loadSafelineConsolePort(install)
	}
	websites, _ := websiteRepo.List()
	for _, website := range websites {
		if website.Type == constant.Stream {
			continue
		}
		brief := dto.GuardSiteBrief{WebsiteID: website.ID, Alias: website.Alias, PrimaryDomain: website.PrimaryDomain}
		if site, ok := loadGuardSite(website.ID); ok {
			brief.Enable = site.Enable
			brief.Mode = site.Mode
		}
		res.Sites = append(res.Sites, brief)
	}
	return res, nil
}

func (s *WebsiteGuardService) UpdateGlobal(req dto.GuardGlobalConfig) error {
	var err error
	if req.IPAllow, err = normalizeGuardIPs(req.IPAllow); err != nil {
		return err
	}
	if req.IPDeny, err = normalizeGuardIPs(req.IPDeny); err != nil {
		return err
	}
	req.Countries = normalizeGuardCountries(req.Countries)
	if req.GeoMode == "" {
		req.GeoMode = "none"
	}
	if req.Enable {
		install, err := getAppInstallByKey(constant.AppOpenresty)
		if err != nil {
			return buserr.New("ErrGuardNoOpenresty")
		}
		if guardWafActive(install) {
			return buserr.New("ErrGuardWafConflict")
		}
	}
	old, _ := settingRepo.GetValueByKey(guardSettingKey)
	content, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err = settingRepo.UpdateOrCreate(guardSettingKey, string(content)); err != nil {
		return err
	}
	if err = syncGuard(); err != nil {
		_ = settingRepo.UpdateOrCreate(guardSettingKey, old)
		_ = syncGuard()
		return err
	}
	return nil
}

func (s *WebsiteGuardService) GetSite(websiteID uint) (*dto.GuardSiteConfig, error) {
	if _, err := websiteRepo.GetFirst(repo.WithByID(websiteID)); err != nil {
		return nil, err
	}
	if site, ok := loadGuardSite(websiteID); ok {
		return guardSiteToDTO(site), nil
	}
	cfg := loadGuardGlobalConfig()
	return &dto.GuardSiteConfig{
		WebsiteID:   websiteID,
		Mode:        cfg.Mode,
		CCEnable:    cfg.CCEnable,
		CCDuration:  cfg.CCDuration,
		CCThreshold: cfg.CCThreshold,
		CCBanTime:   cfg.CCBanTime,
		IPAllow:     []string{},
		IPDeny:      []string{},
		GeoMode:     "none",
		Countries:   []string{},
	}, nil
}

func (s *WebsiteGuardService) UpdateSite(req dto.GuardSiteConfig) error {
	if _, err := websiteRepo.GetFirst(repo.WithByID(req.WebsiteID)); err != nil {
		return err
	}
	allow, err := normalizeGuardIPs(req.IPAllow)
	if err != nil {
		return err
	}
	deny, err := normalizeGuardIPs(req.IPDeny)
	if err != nil {
		return err
	}
	if req.Mode == "" {
		req.Mode = "block"
	}
	if req.GeoMode == "" {
		req.GeoMode = "none"
	}
	if req.Enable && !loadGuardGlobalConfig().Enable {
		return buserr.New("ErrGuardDisabled")
	}
	site, exist := loadGuardSite(req.WebsiteID)
	previous := site
	site.WebsiteID = req.WebsiteID
	site.Enable = req.Enable
	site.Mode = req.Mode
	site.CCEnable = req.CCEnable
	site.CCDuration = req.CCDuration
	site.CCThreshold = req.CCThreshold
	site.CCBanTime = req.CCBanTime
	site.IPAllow = strings.Join(allow, "\n")
	site.IPDeny = strings.Join(deny, "\n")
	site.GeoMode = req.GeoMode
	site.Countries = strings.Join(normalizeGuardCountries(req.Countries), ",")
	if err = global.DB.Save(&site).Error; err != nil {
		return err
	}
	if err = syncGuard(); err != nil {
		if exist {
			_ = global.DB.Save(&previous).Error
		} else {
			_ = global.DB.Delete(&site).Error
		}
		_ = syncGuard()
		return err
	}
	return nil
}

// syncGuardQuietly is called from website lifecycle hooks, which must not
// fail because of the guard.
func syncGuardQuietly() {
	if err := syncGuard(); err != nil {
		global.LOG.Errorf("sync website guard failed: %v", err)
	}
}

type guardFileSnapshot map[string][]byte

func (s guardFileSnapshot) capture(file string) {
	if _, ok := s[file]; ok {
		return
	}
	content, err := os.ReadFile(file)
	if err != nil {
		s[file] = nil
		return
	}
	s[file] = content
}

func (s guardFileSnapshot) restore() {
	for file, content := range s {
		if content == nil {
			_ = os.Remove(file)
			continue
		}
		_ = os.WriteFile(file, content, constant.FilePerm)
	}
}

type guardSiteRender struct {
	Mode      string        `json:"mode"`
	CC        guardCCRender `json:"cc"`
	IPAllow   []string      `json:"ipAllow"`
	IPDeny    []string      `json:"ipDeny"`
	GeoMode   string        `json:"geoMode"`
	Countries []string      `json:"countries"`
}

type guardCCRender struct {
	Enable    bool `json:"enable"`
	Duration  int  `json:"duration"`
	Threshold int  `json:"threshold"`
	BanTime   int  `json:"banTime"`
}

type guardConfigRender struct {
	Detector      string                     `json:"detector,omitempty"`
	LogDir        string                     `json:"logDir"`
	SlowThreshold int                        `json:"slowThreshold"`
	Sites         map[string]guardSiteRender `json:"sites"`
}

func mergeGuardList(a, b []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, item := range list {
			if item != "" && !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	return out
}

// renderGuardConfig builds config.json for the enabled sites and returns the
// countries referenced, which drive the geo map.
func renderGuardConfig(cfg dto.GuardGlobalConfig, websites map[uint]model.Website, sites []model.WebsiteGuard, detector bool) (guardConfigRender, []string) {
	render := guardConfigRender{
		LogDir:        guardContainerLog,
		SlowThreshold: cfg.SlowThreshold,
		Sites:         make(map[string]guardSiteRender),
	}
	if detector {
		render.Detector = "unix:" + guardContainerRoot + "/detector/snserver.sock"
	}
	countrySet := make(map[string]bool)
	for _, site := range sites {
		website, ok := websites[site.WebsiteID]
		if !ok || !site.Enable {
			continue
		}
		item := guardSiteRender{
			Mode: site.Mode,
			CC: guardCCRender{
				Enable:    site.CCEnable,
				Duration:  site.CCDuration,
				Threshold: site.CCThreshold,
				BanTime:   site.CCBanTime,
			},
			IPAllow: mergeGuardList(cfg.IPAllow, splitGuardList(site.IPAllow)),
			IPDeny:  mergeGuardList(cfg.IPDeny, splitGuardList(site.IPDeny)),
			GeoMode: site.GeoMode,
		}
		siteCountries := normalizeGuardCountries(strings.Split(site.Countries, ","))
		if item.GeoMode == "none" || item.GeoMode == "" {
			// Fall back to the global region policy.
			item.GeoMode = cfg.GeoMode
			siteCountries = cfg.Countries
		}
		item.Countries = siteCountries
		if item.Countries == nil {
			item.Countries = []string{}
		}
		if item.GeoMode == "deny" || item.GeoMode == "allow" {
			for _, c := range item.Countries {
				countrySet[c] = true
			}
		}
		render.Sites[website.Alias] = item
	}
	countries := make([]string, 0, len(countrySet))
	for c := range countrySet {
		countries = append(countries, c)
	}
	sort.Strings(countries)
	return render, countries
}

// writeFileIfChanged reports whether the content on disk was replaced.
func writeFileIfChanged(file string, content []byte) (bool, error) {
	if old, err := os.ReadFile(file); err == nil && string(old) == string(content) {
		return false, nil
	}
	return true, os.WriteFile(file, content, constant.FilePerm)
}

func writeGuardLua(root string) (changed bool, err error) {
	err = fs.WalkDir(guardLua.Lua, "lua", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := path.Join(root, p)
		if d.IsDir() {
			return os.MkdirAll(target, constant.DirPerm)
		}
		content, err := guardLua.Lua.ReadFile(p)
		if err != nil {
			return err
		}
		fileChanged, err := writeFileIfChanged(target, content)
		changed = changed || fileChanged
		return err
	})
	return changed, err
}

func renderGuardHTTPConf() []byte {
	return []byte(nginxHTTPConfigHeader +
		"lua_shared_dict guard_cc 20m;\n" +
		"lua_shared_dict guard_ban 10m;\n")
}

// updateGuardSiteDirectives adds or removes the guard hooks of one site.
// It returns whether the file changed.
func updateGuardSiteDirectives(website model.Website, alias string, enable bool, snapshot guardFileSnapshot) (bool, error) {
	confPath := GetWebsiteConfigPath(website)
	content, err := os.ReadFile(confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	config, err := parser.NewStringParser(string(content)).Parse()
	if err != nil {
		return false, err
	}
	config.FilePath = confPath
	servers := config.FindServers()
	if len(servers) == 0 {
		return false, nil
	}
	changed := false
	for _, server := range servers {
		kept := make([]components.IDirective, 0, len(server.Directives))
		hasHooks := false
		for _, dir := range server.Directives {
			params := dir.GetParameters()
			isGuard := (dir.GetName() == "set" && len(params) > 0 && params[0] == guardSiteVar) ||
				((dir.GetName() == "access_by_lua_file" || dir.GetName() == "log_by_lua_file") && len(params) > 0 && strings.HasPrefix(params[0], guardContainerRoot+"/"))
			if isGuard {
				hasHooks = true
				continue
			}
			kept = append(kept, dir)
		}
		if enable {
			// Put the hooks first so rewrite-phase directives such as the
			// HTTPS redirect still see $guard_site.
			hooks := []components.IDirective{
				&components.Directive{Name: "set", Parameters: []string{guardSiteVar, alias}},
				&components.Directive{Name: "access_by_lua_file", Parameters: []string{guardAccessEntry}},
				&components.Directive{Name: "log_by_lua_file", Parameters: []string{guardLogEntry}},
			}
			kept = append(hooks, kept...)
			changed = true
		} else if hasHooks {
			changed = true
		}
		server.Directives = kept
	}
	if !changed {
		return false, nil
	}
	snapshot.capture(confPath)
	if err = nginx.WriteConfig(config, nginx.IndentedStyle); err != nil {
		return false, err
	}
	updated, err := os.ReadFile(confPath)
	return err != nil || string(updated) != string(content), err
}

// syncGuard renders every guard artefact from the database and reloads
// OpenResty, restoring all touched files when nginx -t fails.
func syncGuard() error {
	guardSyncMu.Lock()
	defer guardSyncMu.Unlock()

	install, err := getAppInstallByKey(constant.AppOpenresty)
	if err != nil {
		return nil
	}
	cfg := loadGuardGlobalConfig()
	snapshot := make(guardFileSnapshot)
	httpConfDir := nginxHTTPConfigDir(install)
	httpConfPath := path.Join(httpConfDir, guardHTTPConfName)
	geoConfPath := path.Join(httpConfDir, guardGeoConfName)

	websiteList, _ := websiteRepo.List()
	websites := make(map[uint]model.Website, len(websiteList))
	for _, website := range websiteList {
		if website.Type != constant.Stream {
			websites[website.ID] = website
		}
	}
	var sites []model.WebsiteGuard
	if cfg.Enable {
		if err = global.DB.Find(&sites).Error; err != nil {
			return err
		}
	}
	enabled := make(map[uint]bool)
	for _, site := range sites {
		if site.Enable {
			enabled[site.WebsiteID] = true
		}
	}

	changed := false
	if cfg.Enable && len(enabled) > 0 {
		if guardWafActive(install) {
			return buserr.New("ErrGuardWafConflict")
		}
		includeActive, mainSnapshot, err := ensureNginxHTTPIncludeActive(install)
		if err != nil {
			return err
		}
		if !includeActive {
			return buserr.New("ErrGuardNoHTTPInclude")
		}
		if mainSnapshot != nil {
			snapshot[nginxMainConfigPath(install)] = mainSnapshot
		}
		root := guardRootDir()
		luaChanged, err := writeGuardLua(root)
		if err != nil {
			return err
		}
		changed = changed || luaChanged
		for _, dir := range []string{path.Join(root, "conf"), path.Join(root, "detector"), guardLogDir(install)} {
			if err = os.MkdirAll(dir, constant.DirPerm); err != nil {
				return err
			}
		}
		_, detectorStatus := guardContainerState(safelineDetectorContainer)
		render, countries := renderGuardConfig(cfg, websites, sites, detectorStatus == "running")
		content, err := json.MarshalIndent(render, "", "  ")
		if err != nil {
			return err
		}
		configChanged, err := writeFileIfChanged(path.Join(root, "conf", "config.json"), content)
		if err != nil {
			return err
		}
		changed = changed || configChanged
		geoConf, err := renderGuardGeoConf(countries)
		if err != nil {
			return err
		}
		for file, data := range map[string][]byte{httpConfPath: renderGuardHTTPConf(), geoConfPath: geoConf} {
			if old, readErr := os.ReadFile(file); readErr == nil && string(old) == string(data) {
				continue
			}
			snapshot.capture(file)
			if err = writeNginxFileAtomic(file, data); err != nil {
				snapshot.restore()
				return err
			}
			changed = true
		}
	}

	for id, website := range websites {
		siteChanged, err := updateGuardSiteDirectives(website, website.Alias, cfg.Enable && enabled[id], snapshot)
		if err != nil {
			snapshot.restore()
			return err
		}
		changed = changed || siteChanged
	}
	if !(cfg.Enable && len(enabled) > 0) {
		for _, file := range []string{httpConfPath, geoConfPath} {
			if _, err := os.Stat(file); err == nil {
				snapshot.capture(file)
				_ = os.Remove(file)
				changed = true
			}
		}
	}
	if !changed || install.Status != constant.StatusRunning {
		return nil
	}
	if err = opNginx(install.ContainerName, constant.NginxCheck); err != nil {
		snapshot.restore()
		return buserr.WithErr("ErrGuardNginxCheck", err)
	}
	if err = opNginx(install.ContainerName, constant.NginxReload); err != nil {
		snapshot.restore()
		return err
	}
	return nil
}

// renderGuardGeoConf maps the referenced countries (plus private ranges as
// LAN, which are never region-blocked) to $guard_country.
func renderGuardGeoConf(countries []string) ([]byte, error) {
	var sb strings.Builder
	sb.WriteString(nginxHTTPConfigHeader)
	sb.WriteString("geo $guard_country {\n    default \"\";\n")
	for _, cidr := range []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "::1/128", "fc00::/7", "fe80::/10"} {
		fmt.Fprintf(&sb, "    %s LAN;\n", cidr)
	}
	if len(countries) > 0 {
		wanted := make(map[string]bool, len(countries))
		for _, c := range countries {
			wanted[c] = true
		}
		reader, err := geo.NewGeo()
		if err != nil {
			return nil, buserr.WithErr("ErrGuardGeoDB", err)
		}
		defer reader.Close()
		networks := reader.Networks()
		for networks.Next() {
			var record geo.LocationRes
			subnet, err := networks.Network(&record)
			if err != nil {
				return nil, err
			}
			code := strings.ToUpper(record.Iso)
			if wanted[code] {
				fmt.Fprintf(&sb, "    %s %s;\n", subnet.String(), code)
			}
		}
		if err = networks.Err(); err != nil {
			return nil, err
		}
	}
	sb.WriteString("}\n")
	return []byte(sb.String()), nil
}

var (
	guardCountriesOnce sync.Once
	guardCountries     []dto.GuardCountry
	guardCountriesErr  error
)

func (s *WebsiteGuardService) LoadCountries() ([]dto.GuardCountry, error) {
	guardCountriesOnce.Do(func() {
		reader, err := geo.NewGeo()
		if err != nil {
			guardCountriesErr = err
			return
		}
		defer reader.Close()
		seen := make(map[string]dto.GuardCountry)
		networks := reader.Networks()
		for networks.Next() {
			var record geo.LocationRes
			if _, err := networks.Network(&record); err != nil {
				continue
			}
			code := strings.ToUpper(record.Iso)
			if len(code) != 2 {
				continue
			}
			if _, ok := seen[code]; !ok {
				seen[code] = dto.GuardCountry{Code: code, Zh: record.Country.Zh, En: record.Country.En}
			}
		}
		for _, c := range seen {
			guardCountries = append(guardCountries, c)
		}
		sort.Slice(guardCountries, func(i, j int) bool { return guardCountries[i].Code < guardCountries[j].Code })
	})
	if guardCountriesErr != nil {
		guardCountriesOnce = sync.Once{}
		return nil, guardCountriesErr
	}
	return guardCountries, nil
}
