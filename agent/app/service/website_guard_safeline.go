package service

import (
	"fmt"
	"net"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
	"github.com/1Panel-dev/1Panel/agent/utils/compose"
	"github.com/joho/godotenv"
)

// The SafeLine detector does not work on its own: its supervisor pulls the
// detector config from safeline-mgt and the detection policy is compiled by
// safeline-fvm and pushed by mgt. The guard therefore deploys a trimmed
// SafeLine CE stack (postgres, mgt, fvm, detector, luigi), leaving out the
// tengine gateway and the chaos bot-protection service because OpenResty
// takes their place. Policies, attack logs and the detector's own settings
// stay manageable from the SafeLine console.

const (
	safelineVersion           = "9.4.2"
	safelineMgtContainer      = "safeline-mgt"
	safelineDetectorContainer = "safeline-detector"
	safelineDefaultPort       = 9443
)

var safelineContainers = []string{"safeline-pg", safelineMgtContainer, safelineDetectorContainer, "safeline-luigi", "safeline-fvm"}

const safelineCompose = `# Managed by 1Panel: trimmed SafeLine CE stack used by the website guard.
networks:
  safeline-ce:
    name: safeline-ce
    driver: bridge
    ipam:
      driver: default
      config:
        - gateway: ${SUBNET_PREFIX}.1
          subnet: ${SUBNET_PREFIX}.0/24
    driver_opts:
      com.docker.network.bridge.name: safeline-ce

services:
  postgres:
    container_name: safeline-pg
    restart: always
    image: ${IMAGE_PREFIX}/safeline-postgres${ARCH_SUFFIX}:15.2
    volumes:
      - ${SAFELINE_DIR}/resources/postgres/data:/var/lib/postgresql/data
      - /etc/localtime:/etc/localtime:ro
    environment:
      - POSTGRES_USER=safeline-ce
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
    networks:
      safeline-ce:
        ipv4_address: ${SUBNET_PREFIX}.2
    command: [postgres, -c, max_connections=600]
    healthcheck:
      test: pg_isready -U safeline-ce -d safeline-ce
    labels:
      createdBy: "1Panel"
  mgt:
    container_name: safeline-mgt
    restart: always
    image: ${IMAGE_PREFIX}/safeline-mgt${ARCH_SUFFIX}:${IMAGE_TAG}
    volumes:
      - /etc/localtime:/etc/localtime:ro
      - ${SAFELINE_DIR}/resources/mgt:/app/data
      - ${SAFELINE_DIR}/logs/nginx:/app/log/nginx:z
      - ${SAFELINE_DIR}/resources/sock:/app/sock
      - /var/run:/app/run
    ports:
      - ${MGT_PORT}:1443
    healthcheck:
      test: curl -k -f https://localhost:1443/api/open/health
    environment:
      - MGT_PG=postgres://safeline-ce:${POSTGRES_PASSWORD}@safeline-pg/safeline-ce?sslmode=disable
    depends_on:
      - postgres
      - fvm
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"
    networks:
      safeline-ce:
        ipv4_address: ${SUBNET_PREFIX}.4
    labels:
      createdBy: "1Panel"
  detect:
    container_name: safeline-detector
    restart: always
    image: ${IMAGE_PREFIX}/safeline-detector${ARCH_SUFFIX}:${IMAGE_TAG}
    volumes:
      - ${DETECTOR_DIR}:/resources/detector
      - ${SAFELINE_DIR}/logs/detector:/logs/detector
      - /etc/localtime:/etc/localtime:ro
    environment:
      - LOG_DIR=/logs/detector
    networks:
      safeline-ce:
        ipv4_address: ${SUBNET_PREFIX}.5
    labels:
      createdBy: "1Panel"
  luigi:
    container_name: safeline-luigi
    restart: always
    image: ${IMAGE_PREFIX}/safeline-luigi${ARCH_SUFFIX}:${IMAGE_TAG}
    environment:
      - MGT_IP=${SUBNET_PREFIX}.4
      - LUIGI_PG=postgres://safeline-ce:${POSTGRES_PASSWORD}@safeline-pg/safeline-ce?sslmode=disable
    volumes:
      - /etc/localtime:/etc/localtime:ro
      - ${SAFELINE_DIR}/resources/luigi:/app/data
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"
    depends_on:
      - detect
      - mgt
    networks:
      safeline-ce:
        ipv4_address: ${SUBNET_PREFIX}.7
    labels:
      createdBy: "1Panel"
  fvm:
    container_name: safeline-fvm
    restart: always
    image: ${IMAGE_PREFIX}/safeline-fvm${ARCH_SUFFIX}:${IMAGE_TAG}
    volumes:
      - /etc/localtime:/etc/localtime:ro
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"
    networks:
      safeline-ce:
        ipv4_address: ${SUBNET_PREFIX}.8
    labels:
      createdBy: "1Panel"
`

func guardSafelineDir(install model.AppInstall) string {
	return path.Join(install.GetPath(), "guard-safeline")
}

func loadSafelineConsolePort(install model.AppInstall) int {
	envs, err := godotenv.Read(path.Join(guardSafelineDir(install), ".env"))
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(envs["MGT_PORT"])
	return port
}

// pickSafelineSubnet returns a /24 prefix that no docker network uses yet.
func pickSafelineSubnet() (string, error) {
	out, _ := cmd.NewCommandMgr(cmd.WithTimeout(20*time.Second)).RunWithStdout("sh", "-c", "docker network inspect $(docker network ls -q) --format '{{range .IPAM.Config}}{{.Subnet}} {{end}}'")
	var used []*net.IPNet
	for _, item := range strings.Fields(out) {
		if _, subnet, err := net.ParseCIDR(item); err == nil {
			used = append(used, subnet)
		}
	}
	candidates := []string{"172.22.222", "172.23.222", "172.24.222", "172.25.222", "172.26.222", "172.27.222", "172.28.222", "172.29.222", "172.30.222", "172.31.222", "10.222.222"}
	for _, prefix := range candidates {
		_, mine, _ := net.ParseCIDR(prefix + ".0/24")
		conflict := false
		for _, subnet := range used {
			if subnet.Contains(mine.IP) || mine.Contains(subnet.IP) {
				conflict = true
				break
			}
		}
		if !conflict {
			return prefix, nil
		}
	}
	return "", buserr.New("ErrGuardSafelineSubnet")
}

func safelineContainersExist() bool {
	for _, name := range safelineContainers {
		if exist, _ := guardContainerState(name); exist {
			return true
		}
	}
	return false
}

func (s *WebsiteGuardService) OperateDetector(req dto.GuardDetectorOperate) (string, error) {
	install, err := getAppInstallByKey(constant.AppOpenresty)
	if err != nil {
		return "", buserr.New("ErrGuardNoOpenresty")
	}
	dir := guardSafelineDir(install)
	composePath := path.Join(dir, "docker-compose.yml")
	if req.Operate != "install" {
		if _, err = os.Stat(composePath); err != nil {
			return "", buserr.New("ErrGuardSafelineNotInstalled")
		}
	}
	switch req.Operate {
	case "install":
		return "", installSafeline(install, req)
	case "start":
		_, err = compose.Up(composePath)
	case "restart":
		_, err = compose.Restart(composePath)
	case "stop":
		_, err = compose.Stop(composePath)
	case "remove":
		if _, err = compose.Down(composePath); err != nil {
			return "", err
		}
		if err = os.RemoveAll(dir); err != nil {
			return "", err
		}
		_ = os.Remove(path.Join(guardRootDir(), "detector", "snserver.sock"))
	case "resetAdmin":
		out, err := cmd.NewCommandMgr(cmd.WithTimeout(60*time.Second)).RunWithStdout("docker", "exec", safelineMgtContainer, "/app/mgt-cli", "reset-admin", "--once")
		if err != nil {
			return "", err
		}
		return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, ""), nil
	}
	if err != nil {
		return "", err
	}
	// The detector address in config.json depends on whether it runs.
	syncGuardQuietly()
	return "", nil
}

func installSafeline(install model.AppInstall, req dto.GuardDetectorOperate) error {
	if safelineContainersExist() {
		return buserr.New("ErrGuardSafelineExists")
	}
	port := req.ConsolePort
	if port == 0 {
		port = safelineDefaultPort
	}
	if err := checkPortExist(port); err != nil {
		return err
	}
	subnet, err := pickSafelineSubnet()
	if err != nil {
		return err
	}
	dir := guardSafelineDir(install)
	detectorDir := path.Join(guardRootDir(), "detector")
	archSuffix := ""
	if runtime.GOARCH == "arm64" {
		archSuffix = "-arm"
	}

	safelineTask, err := task.NewTaskWithOps("SafeLine", task.TaskInstall, task.TaskScopeWebsite, req.TaskID, install.ID)
	if err != nil {
		return err
	}
	safelineTask.AddSubTask(i18n.GetMsgByKey("GuardInstallDetector"), func(t *task.Task) error {
		for _, d := range []string{dir, detectorDir, path.Join(dir, "resources"), path.Join(dir, "logs")} {
			if err := os.MkdirAll(d, constant.DirPerm); err != nil {
				return err
			}
		}
		env := map[string]string{
			"SAFELINE_DIR":      dir,
			"DETECTOR_DIR":      detectorDir,
			"IMAGE_TAG":         safelineVersion,
			"IMAGE_PREFIX":      "chaitin",
			"ARCH_SUFFIX":       archSuffix,
			"MGT_PORT":          strconv.Itoa(port),
			"POSTGRES_PASSWORD": "Sl" + common.RandStr(20),
			"SUBNET_PREFIX":     subnet,
		}
		if err := godotenv.Write(env, path.Join(dir, ".env")); err != nil {
			return err
		}
		composePath := path.Join(dir, "docker-compose.yml")
		if err := os.WriteFile(composePath, []byte(safelineCompose), constant.FilePerm); err != nil {
			return err
		}
		cmdMgr := cmd.NewCommandMgr(cmd.WithTask(*t), cmd.WithTimeout(60*time.Minute))
		if err := cmdMgr.Run("docker", "compose", "-f", composePath, "pull"); err != nil {
			return err
		}
		if out, err := compose.Up(composePath); err != nil {
			return fmt.Errorf("%s: %v", out, err)
		}
		t.Log(i18n.GetMsgByKey("GuardWaitSafeline"))
		healthy := false
		for i := 0; i < 90; i++ {
			out, _ := cmd.NewCommandMgr(cmd.WithTimeout(10*time.Second)).RunWithStdout("docker", "inspect", "--format", "{{.State.Health.Status}}", safelineMgtContainer)
			if strings.TrimSpace(out) == "healthy" {
				healthy = true
				break
			}
			time.Sleep(3 * time.Second)
		}
		if !healthy {
			return buserr.New("ErrGuardSafelineUnhealthy")
		}
		if out, err := cmd.NewCommandMgr(cmd.WithTimeout(60*time.Second)).RunWithStdout("docker", "exec", safelineMgtContainer, "/app/mgt-cli", "reset-admin", "--once"); err == nil {
			t.Log(regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(strings.TrimSpace(out), ""))
		}
		return syncGuard()
	}, nil)
	go func() {
		_ = safelineTask.Execute()
	}()
	return nil
}
