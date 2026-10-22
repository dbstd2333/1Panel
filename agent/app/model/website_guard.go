package model

// WebsiteGuard holds the per-site protection settings rendered into the
// OpenResty guard config. List fields are newline separated.
type WebsiteGuard struct {
	BaseModel
	WebsiteID   uint   `gorm:"uniqueIndex;not null" json:"websiteId"`
	Enable      bool   `json:"enable"`
	Mode        string `json:"mode"`
	CCEnable    bool   `json:"ccEnable"`
	CCDuration  int    `json:"ccDuration"`
	CCThreshold int    `json:"ccThreshold"`
	CCBanTime   int    `json:"ccBanTime"`
	IPAllow     string `json:"ipAllow"`
	IPDeny      string `json:"ipDeny"`
	GeoMode     string `json:"geoMode"`
	Countries   string `json:"countries"`
}

// The tables below live in website_monitor.db.

type WebsiteMonitorMinute struct {
	ID         uint    `gorm:"primarykey;AUTO_INCREMENT" json:"id"`
	Site       string  `gorm:"index:idx_wm_minute_site,priority:1" json:"site"`
	Minute     int64   `gorm:"index:idx_wm_minute_site,priority:2;index" json:"minute"`
	Requests   int64   `json:"requests"`
	BytesIn    int64   `json:"bytesIn"`
	BytesOut   int64   `json:"bytesOut"`
	UV         int64   `json:"uv"`
	IP         int64   `json:"ip"`
	S2xx       int64   `json:"s2xx"`
	S3xx       int64   `json:"s3xx"`
	S4xx       int64   `json:"s4xx"`
	S5xx       int64   `json:"s5xx"`
	Blocked    int64   `json:"blocked"`
	RtSum      float64 `json:"rtSum"`
	RtHist     string  `json:"rtHist"`
	TopURL     string  `json:"topURL"`
	TopIP      string  `json:"topIP"`
	TopReferer string  `json:"topReferer"`
	TopUA      string  `json:"topUA"`
	TopCountry string  `json:"topCountry"`
	TopErrURL  string  `json:"topErrURL"`
}

type WebsiteMonitorDay struct {
	ID         uint    `gorm:"primarykey;AUTO_INCREMENT" json:"id"`
	Site       string  `gorm:"index:idx_wm_day_site,priority:1" json:"site"`
	Day        int64   `gorm:"index:idx_wm_day_site,priority:2;index" json:"day"`
	Requests   int64   `json:"requests"`
	BytesIn    int64   `json:"bytesIn"`
	BytesOut   int64   `json:"bytesOut"`
	UV         int64   `json:"uv"`
	IP         int64   `json:"ip"`
	S2xx       int64   `json:"s2xx"`
	S3xx       int64   `json:"s3xx"`
	S4xx       int64   `json:"s4xx"`
	S5xx       int64   `json:"s5xx"`
	Blocked    int64   `json:"blocked"`
	RtSum      float64 `json:"rtSum"`
	RtHist     string  `json:"rtHist"`
	TopURL     string  `json:"topURL"`
	TopIP      string  `json:"topIP"`
	TopReferer string  `json:"topReferer"`
	TopUA      string  `json:"topUA"`
	TopCountry string  `json:"topCountry"`
	TopErrURL  string  `json:"topErrURL"`
	UVSketch   []byte  `json:"-"`
	IPSketch   []byte  `json:"-"`
}

type WebsiteMonitorSlow struct {
	ID          uint    `gorm:"primarykey;AUTO_INCREMENT" json:"id"`
	Site        string  `gorm:"index" json:"site"`
	Time        int64   `gorm:"index" json:"time"`
	Method      string  `json:"method"`
	URI         string  `json:"uri"`
	IP          string  `json:"ip"`
	Status      int     `json:"status"`
	RequestTime float64 `json:"requestTime"`
}

type WebsiteGuardEvent struct {
	ID      uint   `gorm:"primarykey;AUTO_INCREMENT" json:"id"`
	Site    string `gorm:"index" json:"site"`
	Time    int64  `gorm:"index" json:"time"`
	IP      string `gorm:"index" json:"ip"`
	Country string `json:"country"`
	Method  string `json:"method"`
	Host    string `json:"host"`
	URI     string `json:"uri"`
	Type    string `gorm:"index" json:"type"`
	Action  string `json:"action"`
	EventID string `json:"eventId"`
	UA      string `json:"ua"`
}
