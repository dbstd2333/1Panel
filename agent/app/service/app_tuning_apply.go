package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/compose"
)

const (
	pgBouncerImage       = "edoburu/pgbouncer:v1.26.0-p0"
	pgBouncerServiceName = "pgbouncer"
	pgBouncerDir         = "pgbouncer"
	// uid of the postgres user inside the pgbouncer image
	pgBouncerUID = 70

	pgTuningBlockStart = "# PostgreSQL tuning by 1Panel"
	pgTuningBlockEnd   = "# End PostgreSQL tuning by 1Panel"
)

func paramString(params map[string]interface{}, key string) string {
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func paramBool(params map[string]interface{}, key string) bool {
	value, ok := params[key]
	if !ok || value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	default:
		b, _ := strconv.ParseBool(fmt.Sprintf("%v", v))
		return b
	}
}

func pgBouncerEnabled(appKey string, params map[string]interface{}) bool {
	return appKey == "postgresql" && paramBool(params, PgBouncerEnableKey)
}

// normalizeTuningParams makes the stored env self-describing so upgrades and
// password changes can re-apply the same choices later.
func normalizeTuningParams(appKey string, params map[string]interface{}) {
	if !isTuningApp(appKey) {
		return
	}
	params[TuneTemplateKey] = normalizeTuningTemplate(paramString(params, TuneTemplateKey))
	if appKey != "postgresql" {
		return
	}
	enabled := paramBool(params, PgBouncerEnableKey)
	params[PgBouncerEnableKey] = strconv.FormatBool(enabled)
	if !enabled {
		delete(params, PgBouncerPortKey)
		delete(params, PgBouncerPoolModeKey)
		return
	}
	if mode := paramString(params, PgBouncerPoolModeKey); mode != "session" {
		params[PgBouncerPoolModeKey] = "transaction"
	}
}

// injectPgBouncerService adds a pgbouncer sidecar in front of the postgres
// service. It runs before addDockerComposeCommonParam so the image prefix of
// multi-node setups is applied to it too.
func injectPgBouncerService(servicesMap map[string]interface{}, serviceName string) {
	servicesMap[pgBouncerServiceName] = map[string]interface{}{
		"image":          pgBouncerImage,
		"container_name": "${CONTAINER_NAME}-pgbouncer",
		"restart":        "always",
		"depends_on": map[string]interface{}{
			serviceName: map[string]interface{}{"condition": "service_healthy"},
		},
		"ports":    []interface{}{"${HOST_IP}:${" + PgBouncerPortKey + "}:6432"},
		"volumes":  []interface{}{"./" + pgBouncerDir + ":/etc/pgbouncer"},
		"networks": []interface{}{"1panel-network"},
		"labels":   map[string]interface{}{"createdBy": "Apps"},
	}
}

func renderPgBouncerIni(params map[string]interface{}) string {
	tpl := paramString(params, TuneTemplateKey)
	defaultPool, reservePool, maxDBConn, maxClientConn := pgBouncerPoolSize(tpl)
	poolMode := paramString(params, PgBouncerPoolModeKey)
	if poolMode != "session" {
		poolMode = "transaction"
	}
	host := paramString(params, constant.ContainerName)
	rootUser := paramString(params, "PANEL_DB_ROOT_USER")
	var sb strings.Builder
	sb.WriteString("# Managed by 1Panel. Manual changes will be overwritten.\n")
	sb.WriteString("[databases]\n")
	fmt.Fprintf(&sb, "* = host=%s port=5432\n\n", host)
	sb.WriteString("[pgbouncer]\n")
	lines := [][2]string{
		{"listen_addr", "0.0.0.0"},
		{"listen_port", "6432"},
		{"pool_mode", poolMode},
		{"auth_type", "scram-sha-256"},
		{"auth_file", "/etc/pgbouncer/userlist.txt"},
		{"auth_user", rootUser},
		{"auth_query", "SELECT usename, passwd FROM pg_shadow WHERE usename=$1"},
		{"admin_users", rootUser},
		{"stats_users", rootUser},
		{"server_reset_query", "DISCARD ALL"},
		{"max_client_conn", strconv.Itoa(maxClientConn)},
		{"default_pool_size", strconv.Itoa(defaultPool)},
		{"min_pool_size", "0"},
		{"reserve_pool_size", strconv.Itoa(reservePool)},
		{"reserve_pool_timeout", "1"},
		{"max_db_connections", strconv.Itoa(maxDBConn)},
		{"server_lifetime", "600"},
		{"max_prepared_statements", "256"},
		{"ignore_startup_parameters", "extra_float_digits"},
		{"log_connections", "0"},
		{"log_disconnections", "0"},
	}
	for _, line := range lines {
		fmt.Fprintf(&sb, "%-26s= %s\n", line[0], line[1])
	}
	return sb.String()
}

func writePgBouncerConfig(appInstall *model.AppInstall, params map[string]interface{}) error {
	dir := path.Join(appInstall.GetPath(), pgBouncerDir)
	if err := os.MkdirAll(dir, constant.DirPerm); err != nil {
		return err
	}
	iniPath := path.Join(dir, "pgbouncer.ini")
	if err := os.WriteFile(iniPath, []byte(renderPgBouncerIni(params)), 0644); err != nil {
		return err
	}
	if err := writePgBouncerUserlist(dir, paramString(params, "PANEL_DB_ROOT_USER"), paramString(params, "PANEL_DB_ROOT_PASSWORD")); err != nil {
		return err
	}
	_ = os.Chown(dir, pgBouncerUID, pgBouncerUID)
	_ = os.Chown(iniPath, pgBouncerUID, pgBouncerUID)
	return nil
}

func writePgBouncerUserlist(dir, user, password string) error {
	quote := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	userlist := path.Join(dir, "userlist.txt")
	if err := os.WriteFile(userlist, []byte(quote(user)+" "+quote(password)+"\n"), 0600); err != nil {
		return err
	}
	_ = os.Chown(userlist, pgBouncerUID, pgBouncerUID)
	return nil
}

// refreshPgBouncerAuth keeps the auth_user credentials in sync after the
// root password of a local postgresql install changes.
func refreshPgBouncerAuth(appInstall model.AppInstall, password string) {
	dir := path.Join(appInstall.GetPath(), pgBouncerDir)
	if _, err := os.Stat(path.Join(dir, "pgbouncer.ini")); err != nil {
		return
	}
	user := ""
	if envs, err := loadAppInstallEnv(appInstall); err == nil {
		user = paramString(envs, "PANEL_DB_ROOT_USER")
	}
	if user == "" {
		return
	}
	if err := writePgBouncerUserlist(dir, user, password); err != nil {
		global.LOG.Errorf("update pgbouncer userlist for %s failed: %v", appInstall.Name, err)
		return
	}
	if _, err := compose.Restart(appInstall.GetComposePath()); err != nil {
		global.LOG.Errorf("restart %s after pgbouncer auth update failed: %v", appInstall.Name, err)
	}
}

func applyRedisTuning(appInstall *model.AppInstall, items []tuningItem) error {
	if len(items) == 0 {
		return nil
	}
	confs := make([]redisConfig, 0, len(items))
	for _, item := range items {
		confs = append(confs, redisConfig{key: item.Key, value: item.Value})
	}
	return confSetByPath(path.Join(appInstall.GetPath(), "conf", "redis.conf"), "", confs)
}

func pgConfPath(appInstall *model.AppInstall) string {
	if appMajorVersion(appInstall.Version) >= 18 {
		return path.Join(appInstall.GetPath(), "data", "18", "docker", "postgresql.conf")
	}
	return path.Join(appInstall.GetPath(), "data", "postgresql.conf")
}

// renderPgTuningBlock drops any previous managed block and appends a fresh
// one at the end of postgresql.conf, so its values win over the defaults
// written by initdb while the whole file stays editable from the conf page.
func renderPgTuningBlock(content string, items []tuningItem) string {
	if start := strings.Index(content, pgTuningBlockStart); start >= 0 {
		end := strings.Index(content[start:], pgTuningBlockEnd)
		if end >= 0 {
			rest := content[start+end+len(pgTuningBlockEnd):]
			content = content[:start] + strings.TrimLeft(rest, "\n")
		}
	}
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(content, "\n"))
	sb.WriteString("\n\n" + pgTuningBlockStart + "\n")
	for _, item := range items {
		value := item.Value
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			value = "'" + value + "'"
		}
		fmt.Fprintf(&sb, "%s = %s\n", item.Key, value)
	}
	sb.WriteString(pgTuningBlockEnd + "\n")
	return sb.String()
}

// applyPgTuning waits for initdb to finish, writes the tuning block and
// restarts the database. postgresql.conf only exists after the first start,
// which is why this runs after compose up instead of before it.
func applyPgTuning(t *task.Task, appInstall *model.AppInstall, items []tuningItem) error {
	if len(items) == 0 {
		return nil
	}
	confPath := pgConfPath(appInstall)
	user := ""
	if envs, err := loadAppInstallEnv(*appInstall); err == nil {
		user = paramString(envs, "PANEL_DB_ROOT_USER")
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if _, err := os.Stat(confPath); err == nil {
			if err := cmd.RunDockerExec(10*time.Second, appInstall.ContainerName, "pg_isready", "-h", "127.0.0.1", "-p", "5432", "-q", "-U", user); err == nil {
				break
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait for postgresql ready timeout")
		}
		time.Sleep(3 * time.Second)
	}
	content, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	if err = os.WriteFile(confPath, []byte(renderPgTuningBlock(string(content), items)), constant.FilePerm); err != nil {
		return err
	}
	t.Log(fmt.Sprintf("write %d tuning parameters to %s", len(items), confPath))
	_, err = compose.Restart(appInstall.GetComposePath())
	return err
}

func loadAppInstallEnv(appInstall model.AppInstall) (map[string]interface{}, error) {
	envs := make(map[string]interface{})
	if err := json.Unmarshal([]byte(appInstall.Env), &envs); err != nil {
		return nil, err
	}
	return envs, nil
}
