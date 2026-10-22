package dto

// GuardGlobalConfig is stored as JSON in the GuardConfig setting. IP lists
// and countries apply to every guarded site on top of the site's own lists.
type GuardGlobalConfig struct {
	Enable        bool     `json:"enable"`
	IPAllow       []string `json:"ipAllow"`
	IPDeny        []string `json:"ipDeny"`
	GeoMode       string   `json:"geoMode" validate:"omitempty,oneof=none deny allow"`
	Countries     []string `json:"countries"`
	SlowThreshold int      `json:"slowThreshold" validate:"min=0"`
	MinuteDays    int      `json:"minuteDays" validate:"min=1,max=30"`
	DayDays       int      `json:"dayDays" validate:"min=1,max=365"`
	EventDays     int      `json:"eventDays" validate:"min=1,max=365"`

	// Defaults copied into a site the first time protection is enabled on it.
	Mode        string `json:"mode" validate:"omitempty,oneof=block monitor off"`
	CCEnable    bool   `json:"ccEnable"`
	CCDuration  int    `json:"ccDuration" validate:"min=0"`
	CCThreshold int    `json:"ccThreshold" validate:"min=0"`
	CCBanTime   int    `json:"ccBanTime" validate:"min=0"`
}

type GuardStatus struct {
	OpenrestyInstalled bool              `json:"openrestyInstalled"`
	OpenrestyRunning   bool              `json:"openrestyRunning"`
	WafConflict        bool              `json:"wafConflict"`
	DetectorInstalled  bool              `json:"detectorInstalled"`
	DetectorStatus     string            `json:"detectorStatus"`
	MgtStatus          string            `json:"mgtStatus"`
	ConsolePort        int               `json:"consolePort"`
	SafelineVersion    string            `json:"safelineVersion"`
	Config             GuardGlobalConfig `json:"config"`
	Sites              []GuardSiteBrief  `json:"sites"`
}

type GuardSiteBrief struct {
	WebsiteID     uint   `json:"websiteId"`
	Alias         string `json:"alias"`
	PrimaryDomain string `json:"primaryDomain"`
	Enable        bool   `json:"enable"`
	Mode          string `json:"mode"`
}

type GuardSiteConfig struct {
	WebsiteID   uint     `json:"websiteId" validate:"required"`
	Enable      bool     `json:"enable"`
	Mode        string   `json:"mode" validate:"omitempty,oneof=block monitor off"`
	CCEnable    bool     `json:"ccEnable"`
	CCDuration  int      `json:"ccDuration" validate:"min=0"`
	CCThreshold int      `json:"ccThreshold" validate:"min=0"`
	CCBanTime   int      `json:"ccBanTime" validate:"min=0"`
	IPAllow     []string `json:"ipAllow"`
	IPDeny      []string `json:"ipDeny"`
	GeoMode     string   `json:"geoMode" validate:"omitempty,oneof=none deny allow"`
	Countries   []string `json:"countries"`
}

type GuardDetectorOperate struct {
	Operate     string `json:"operate" validate:"required,oneof=install start stop restart remove resetAdmin"`
	TaskID      string `json:"taskID"`
	ConsolePort int    `json:"consolePort"`
}

type GuardEventSearch struct {
	PageInfo
	Site      string `json:"site"`
	Type      string `json:"type"`
	IP        string `json:"ip"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
}

type GuardEventStats struct {
	Total  int64            `json:"total"`
	ByType map[string]int64 `json:"byType"`
	BySite map[string]int64 `json:"bySite"`
	TopIP  []GuardCount     `json:"topIP"`
	Trend  []GuardCount     `json:"trend"`
}

type GuardCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type GuardCountry struct {
	Code string `json:"code"`
	Zh   string `json:"zh"`
	En   string `json:"en"`
}

type WebsiteMonitorReq struct {
	Site      string `json:"site"`
	StartTime int64  `json:"startTime" validate:"required"`
	EndTime   int64  `json:"endTime" validate:"required"`
}

type WebsiteMonitorOverview struct {
	Requests int64   `json:"requests"`
	UV       int64   `json:"uv"`
	IP       int64   `json:"ip"`
	BytesIn  int64   `json:"bytesIn"`
	BytesOut int64   `json:"bytesOut"`
	S2xx     int64   `json:"s2xx"`
	S3xx     int64   `json:"s3xx"`
	S4xx     int64   `json:"s4xx"`
	S5xx     int64   `json:"s5xx"`
	Blocked  int64   `json:"blocked"`
	AvgRt    float64 `json:"avgRt"`
	P95Rt    float64 `json:"p95Rt"`
	PeakQPS  float64 `json:"peakQps"`
}

type WebsiteMonitorPoint struct {
	Time     int64   `json:"time"`
	Requests int64   `json:"requests"`
	UV       int64   `json:"uv"`
	IP       int64   `json:"ip"`
	BytesIn  int64   `json:"bytesIn"`
	BytesOut int64   `json:"bytesOut"`
	S2xx     int64   `json:"s2xx"`
	S3xx     int64   `json:"s3xx"`
	S4xx     int64   `json:"s4xx"`
	S5xx     int64   `json:"s5xx"`
	Blocked  int64   `json:"blocked"`
	QPS      float64 `json:"qps"`
	AvgRt    float64 `json:"avgRt"`
	P95Rt    float64 `json:"p95Rt"`
}

type WebsiteMonitorTrend struct {
	Interval int64                 `json:"interval"`
	Points   []WebsiteMonitorPoint `json:"points"`
}

type WebsiteMonitorTop struct {
	URL     []GuardCount `json:"url"`
	IP      []GuardCount `json:"ip"`
	Referer []GuardCount `json:"referer"`
	UA      []GuardCount `json:"ua"`
	Country []GuardCount `json:"country"`
	ErrURL  []GuardCount `json:"errUrl"`
}
