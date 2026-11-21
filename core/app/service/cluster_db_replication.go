package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
)

// ClusterDbReplicationService manages cross-server one-way database
// replication topologies. Automated steps only use existing agent endpoints
// reached through the cluster node dispatcher (config-file read/write,
// variables update, backup/recover, connection check, file transfer).
// Steps with no existing endpoint are explicit manual steps with exact SQL:
//
//	MySQL:  CREATE USER + GRANT REPLICATION SLAVE (the panel user API
//	        refuses global privileges), CHANGE REPLICATION SOURCE TO,
//	        START/STOP REPLICA, SHOW REPLICA STATUS.
//	PG:     CREATE ROLE ... REPLICATION, CREATE PUBLICATION,
//	        pg_create_logical_replication_slot, CREATE SUBSCRIPTION.
//
// The panel therefore never pretends replication is running: replica lag
// is reported as unknown (—) and status "running" only means the user
// confirmed the manual start SQL executed.
type ClusterDbReplicationService struct {
	nodes *ClusterNodeService
	repo  repo.IClusterDbReplicationRepo
	sync  *ClusterSyncService
}

type IClusterDbReplicationService interface {
	List() ([]dto.ClusterDbReplicationInfo, error)
	Get(id uint) (dto.ClusterDbReplicationInfo, error)
	Create(req dto.ClusterDbReplicationCreate) (uint, error)
	Update(req dto.ClusterDbReplicationUpdate) error
	Delete(id uint) error
	ListInstances(req dto.ClusterDbInstancesReq) ([]dto.ClusterDbInstance, error)
	ListSchemas(req dto.ClusterDbSchemasReq) ([]dto.ClusterDbSchema, error)
	Preflight(id uint) ([]dto.ClusterDbPreflightCheck, error)
	ManualSteps(id uint) ([]dto.ClusterDbManualStep, error)
	MarkManualStepsDone(id uint, keys []string) error
	ConfigureMaster(id uint) error
	StartSnapshot(id uint) error
	ConfirmReplicationStarted(req dto.ClusterDbConfirmReq) error
	RefreshStatus(id uint) (dto.ClusterDbReplicationInfo, error)
	PromoteSteps(id uint) ([]dto.ClusterDbManualStep, error)
	Promote(req dto.ClusterDbPromoteReq) (dto.ClusterDbFailoverInfo, error)
	ListFailovers(id uint) ([]dto.ClusterDbFailoverInfo, error)
}

func NewIClusterDbReplicationService() IClusterDbReplicationService {
	return &ClusterDbReplicationService{
		nodes: NewIClusterNodeService().(*ClusterNodeService),
		repo:  repo.NewIClusterDbReplicationRepo(),
		sync:  NewIClusterSyncService().(*ClusterSyncService),
	}
}

// maxSnapshotBytes is the streaming guard for snapshot transfers (5 GiB).
// The resource-sync 200 MB cap does not apply here: database dumps are
// routinely larger. Content is streamed, never buffered in memory.
const maxSnapshotBytes = 5 * 1024 * 1024 * 1024

// snapshotPollTimeout bounds how long the background snapshot job waits
// for the agent backup / recover tasks.
const snapshotPollTimeout = 30 * time.Minute

const snapshotPollInterval = 5 * time.Second

func (s *ClusterDbReplicationService) node(id uint) (model.ClusterNode, error) {
	return s.sync.resolveNode(id)
}

func (s *ClusterDbReplicationService) callNode(node model.ClusterNode, method, path string, body interface{}, out interface{}, timeout time.Duration) error {
	return s.sync.callNodeJSON(node, method, path, body, out, timeout)
}

func (s *ClusterDbReplicationService) getRep(id uint) (model.ClusterDbReplication, error) {
	rep, err := s.repo.Get(id)
	if err != nil || rep.ID == 0 {
		return rep, buserr.New("ErrRecordNotFound")
	}
	return rep, nil
}

func encryptReplPassword(password string, id uint) (string, error) {
	key, err := encrypt.APIKeyEncryptionKey()
	if err != nil {
		return "", err
	}
	return encrypt.EncryptAPIKey(password, "dbrepl-"+strconv.FormatUint(uint64(id), 10), key)
}

func decryptReplPassword(rep model.ClusterDbReplication) (string, error) {
	key, err := encrypt.APIKeyEncryptionKey()
	if err != nil {
		return "", err
	}
	return encrypt.DecryptAPIKey(rep.ReplPasswordCipher, "dbrepl-"+strconv.FormatUint(uint64(rep.ID), 10), key)
}

func replInfo(rep model.ClusterDbReplication) dto.ClusterDbReplicationInfo {
	return dto.ClusterDbReplicationInfo{
		ID: rep.ID, Name: rep.Name, DBType: rep.DBType,
		MasterNodeID: rep.MasterNodeID, MasterName: rep.MasterName,
		MasterInstance: rep.MasterInstance, MasterHost: rep.MasterHost,
		MasterPort: rep.MasterPort, Database: rep.Database,
		ReplicaNodeID: rep.ReplicaNodeID, ReplicaName: rep.ReplicaName,
		ReplicaInstance: rep.ReplicaInstance, ReplicaDatabase: rep.ReplicaDatabase,
		ReplUsername: rep.ReplUsername, Status: rep.Status,
		SetupStep: rep.SetupStep, ManualSteps: rep.ManualSteps,
		Preflight:      rep.Preflight,
		MasterServerID: rep.MasterServerID, ReplicaServerID: rep.ReplicaServerID,
		MasterBinlogFile: rep.MasterBinlogFile, MasterBinlogPosition: rep.MasterBinlogPosition,
		SnapshotStatus: rep.SnapshotStatus, SnapshotDetail: rep.SnapshotDetail,
		LastDelaySeconds: rep.LastDelaySeconds, LastError: rep.LastError,
		LastCheckedAt: rep.LastCheckedAt, LastConfirmedAt: rep.LastConfirmedAt,
		FailoverSuggested: rep.FailoverSuggested,
		CreatedAt:         rep.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:         rep.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

var dbIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_$]+$`)

func validDBIdentifier(name string) bool {
	return len(name) > 0 && len(name) <= 64 && dbIdentifierPattern.MatchString(name)
}

// quoteMysqlIdent backtick-quotes a validated identifier.
func quoteMysqlIdent(name string) (string, error) {
	if !validDBIdentifier(name) {
		return "", fmt.Errorf("invalid identifier %q", name)
	}
	return "`" + name + "`", nil
}

// quotePgIdent double-quotes a validated identifier.
func quotePgIdent(name string) (string, error) {
	if !validDBIdentifier(name) {
		return "", fmt.Errorf("invalid identifier %q", name)
	}
	return `"` + name + `"`, nil
}

// quoteSQLString single-quotes a string literal, doubling embedded quotes.
func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func randomTaskID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func isMysqlType(dbType string) bool {
	return dbType == dto.ClusterDbTypeMysql || dbType == dto.ClusterDbTypeMariadb
}

func isPgType(dbType string) bool { return dbType == dto.ClusterDbTypePostgresql }

// instanceConfType maps a panel database type to the config-file type
// accepted by /api/v2/databases/common/load/file.
func instanceConfType(dbType string) string {
	switch dbType {
	case dto.ClusterDbTypeMysql:
		return "mysql-conf"
	case dto.ClusterDbTypeMariadb:
		return "mariadb-conf"
	case dto.ClusterDbTypePostgresql:
		return "postgresql-conf"
	}
	return ""
}

// instanceListType maps a panel database type to the comma list accepted
// by GET /api/v2/databases/db/list/:type.
func instanceListType(dbType string) string {
	switch dbType {
	case dto.ClusterDbTypeMysql:
		return "mysql,mariadb,mysql-cluster"
	case dto.ClusterDbTypeMariadb:
		return "mysql,mariadb,mysql-cluster"
	case dto.ClusterDbTypePostgresql:
		return "postgresql,postgresql-cluster"
	}
	return dbType
}

// ---------- config file parsing ----------

// parseMyCnf returns the [mysqld] section parameters of a my.cnf file.
// Later duplicate keys win; comment lines (# or ;) are ignored.
func parseMyCnf(content string) map[string]string {
	return parseIniSection(content, "[mysqld]")
}

// parsePgConf returns postgresql.conf parameters (key = value, # comments).
func parsePgConf(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "'\"")
		if idx := strings.Index(value, "#"); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
			value = strings.Trim(value, "'\"")
		}
		if key != "" {
			out[key] = value
		}
	}
	return out
}

func parseIniSection(content, group string) map[string]string {
	out := map[string]string{}
	inGroup := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inGroup = trimmed == group
			continue
		}
		if !inGroup || trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		} else if len(parts) == 1 && parts[0] != "" {
			out[strings.TrimSpace(parts[0])] = ""
		}
	}
	return out
}

// setMyCnfParam sets (or adds) one key=value line in the [mysqld] section,
// preserving the rest of the file byte-for-byte.
func setMyCnfParam(content, key, value string) string {
	lines := strings.Split(content, "\n")
	group := "[mysqld]"
	inGroup := false
	hasGroup := false
	hasKey := false
	var out []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			if inGroup && !hasKey {
				out = append(out, key+"="+value)
				hasKey = true
			}
			inGroup = trimmed == group
			if inGroup {
				hasGroup = true
			}
			out = append(out, line)
			continue
		}
		if inGroup && (strings.HasPrefix(trimmed, key+"=") || strings.HasPrefix(trimmed, "# "+key+"=") || strings.HasPrefix(trimmed, ";"+key+"=")) {
			out = append(out, key+"="+value)
			hasKey = true
			continue
		}
		out = append(out, line)
		if inGroup && i == len(lines)-1 && !hasKey {
			out = append(out, key+"="+value)
			hasKey = true
		}
	}
	if !hasGroup {
		out = append(out, group, key+"="+value)
	} else if !hasKey {
		out = append(out, key+"="+value)
	}
	return strings.Join(out, "\n")
}

// setPgConfParam sets (or adds) one key = value line in postgresql.conf.
func setPgConfParam(content, key, value string) string {
	lines := strings.Split(content, "\n")
	hasKey := false
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			out = append(out, key+" = "+value)
			hasKey = true
			continue
		}
		out = append(out, line)
	}
	if !hasKey {
		out = append(out, key+" = "+value)
	}
	return strings.Join(out, "\n")
}

// parseMajorVersion extracts the leading major version number ("8.0.36"
// -> 8, "16.2" -> 16, "10.11.7-MariaDB" -> 10).
func parseMajorVersion(version string) int {
	version = strings.TrimSpace(version)
	major := 0
	for _, ch := range version {
		if ch >= '0' && ch <= '9' {
			major = major*10 + int(ch-'0')
		} else {
			break
		}
	}
	return major
}

// ---------- manual SQL generation ----------

// mysqlReplUserSQL is the exact manual SQL to create the replication
// account on the master. REPLICATION SLAVE is a global privilege, which
// the panel user API refuses, so this is always manual.
func mysqlReplUserSQL(username, host, password string) (string, error) {
	if !validDBIdentifier(username) {
		return "", fmt.Errorf("invalid replication username %q", username)
	}
	if host != "%" && !validDBIdentifier(strings.ReplaceAll(host, ".", "")) && host != "" {
		return "", fmt.Errorf("invalid replication host %q", host)
	}
	if host == "" {
		host = "%"
	}
	return fmt.Sprintf("CREATE USER %s@%s IDENTIFIED BY %s;\nGRANT REPLICATION SLAVE ON *.* TO %s@%s;\nFLUSH PRIVILEGES;",
		quoteSQLString(username), quoteSQLString(host), quoteSQLString(password),
		quoteSQLString(username), quoteSQLString(host)), nil
}

// mysqlChangeSourceSQL is the manual start-replication SQL for the replica.
// SOURCE_AUTO_POSITION=1 (GTID) needs no binlog file/position.
func mysqlChangeSourceSQL(host string, port uint, username, password string) (string, error) {
	if strings.TrimSpace(host) == "" || port == 0 {
		return "", fmt.Errorf("master database address is required")
	}
	if !validDBIdentifier(username) {
		return "", fmt.Errorf("invalid replication username %q", username)
	}
	return fmt.Sprintf("CHANGE REPLICATION SOURCE TO\n  SOURCE_HOST=%s,\n  SOURCE_PORT=%d,\n  SOURCE_USER=%s,\n  SOURCE_PASSWORD=%s,\n  SOURCE_AUTO_POSITION=1,\n  GET_SOURCE_PUBLIC_KEY=1;\nSTART REPLICA;\nSHOW REPLICA STATUS\\G",
		quoteSQLString(host), port, quoteSQLString(username), quoteSQLString(password)), nil
}

func pgReplRoleSQL(username, password string) (string, error) {
	ident, err := quotePgIdent(username)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CREATE ROLE %s WITH LOGIN REPLICATION PASSWORD %s;",
		ident, quoteSQLString(password)), nil
}

func pgPublicationSQL(publication, database string) (string, error) {
	pub, err := quotePgIdent(publication)
	if err != nil {
		return "", err
	}
	// FOR ALL TABLES replicates every current table of the database; tables
	// created later need ALTER PUBLICATION ... ADD TABLE.
	return fmt.Sprintf("-- 在主库的 %s 库执行（需先切换到该库）：\nCREATE PUBLICATION %s FOR ALL TABLES;", database, pub), nil
}

func pgSlotSQL(slot, database string) (string, error) {
	// Validate the slot name as an identifier; it is passed as a string
	// literal to pg_create_logical_replication_slot.
	if _, err := quotePgIdent(slot); err != nil {
		return "", err
	}
	return fmt.Sprintf("-- 在主库的 %s 库执行：\nSELECT pg_create_logical_replication_slot(%s, 'pgoutput');", database, quoteSQLString(slot)), nil
}

func pgSubscriptionSQL(subscription, host string, port uint, database, username, password, publication, slot string) (string, error) {
	sub, err := quotePgIdent(subscription)
	if err != nil {
		return "", err
	}
	pub, err := quotePgIdent(publication)
	if err != nil {
		return "", err
	}
	conn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s", host, port, database, username, password)
	return fmt.Sprintf("-- 在从库的 %s 库执行（数据已由快照恢复，copy_data=false）：\nCREATE SUBSCRIPTION %s CONNECTION %s PUBLICATION %s WITH (copy_data=false, slot_name=%s, create_slot=false);",
		database, sub, quoteSQLString(conn), pub, quoteSQLString(slot)), nil
}

func publicationName(rep model.ClusterDbReplication) string {
	return "cluster_repl_" + strconv.FormatUint(uint64(rep.ID), 10)
}

func subscriptionName(rep model.ClusterDbReplication) string {
	return "cluster_repl_" + strconv.FormatUint(uint64(rep.ID), 10)
}

func slotName(rep model.ClusterDbReplication) string {
	return "cluster_repl_" + strconv.FormatUint(uint64(rep.ID), 10)
}

// ---------- agent endpoint wrappers (via the node dispatcher) ----------

type agentPage struct {
	Total int64             `json:"total"`
	Items []json.RawMessage `json:"items"`
}

// listInstances returns the installed database instances on a node.
func (s *ClusterDbReplicationService) listInstances(node model.ClusterNode, dbType string) ([]dto.ClusterDbInstance, error) {
	var out []struct {
		ID      uint   `json:"id"`
		Name    string `json:"database"`
		Type    string `json:"type"`
		Version string `json:"version"`
		Address string `json:"address"`
	}
	if err := s.callNode(node, http.MethodGet, "/api/v2/databases/db/list/"+instanceListType(dbType), nil, &out, 30*time.Second); err != nil {
		return nil, err
	}
	res := make([]dto.ClusterDbInstance, 0, len(out))
	for _, item := range out {
		res = append(res, dto.ClusterDbInstance{ID: item.ID, Name: item.Name, Type: item.Type, Version: item.Version})
	}
	return res, nil
}

// listSchemas returns the schema names inside one instance.
func (s *ClusterDbReplicationService) listSchemas(node model.ClusterNode, dbType, instance string) ([]string, error) {
	path := "/api/v2/databases/search"
	if isPgType(dbType) {
		path = "/api/v2/databases/pg/search"
	}
	var page agentPage
	body := map[string]interface{}{"database": instance, "page": 1, "pageSize": 500, "orderBy": "createdAt", "order": "null"}
	if err := s.callNode(node, http.MethodPost, path, body, &page, 30*time.Second); err != nil {
		return nil, err
	}
	var names []string
	for _, raw := range page.Items {
		var item struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &item) == nil && item.Name != "" {
			names = append(names, item.Name)
		}
	}
	return names, nil
}

// loadBaseInfo returns the published host port of an instance.
func (s *ClusterDbReplicationService) loadBaseInfo(node model.ClusterNode, dbType, instance string) (uint, error) {
	var info struct {
		Port uint `json:"port"`
	}
	body := map[string]interface{}{"type": dbType, "name": instance}
	if err := s.callNode(node, http.MethodPost, "/api/v2/databases/common/info", body, &info, 30*time.Second); err != nil {
		return 0, err
	}
	return info.Port, nil
}

// loadConf returns the raw config file of an instance.
func (s *ClusterDbReplicationService) loadConf(node model.ClusterNode, dbType, instance string) (string, error) {
	var content string
	body := map[string]interface{}{"type": instanceConfType(dbType), "name": instance}
	if err := s.callNode(node, http.MethodPost, "/api/v2/databases/common/load/file", body, &content, 30*time.Second); err != nil {
		return "", err
	}
	return content, nil
}

// loadMysqlStatus returns the master binlog file/position ("OFF"/"OFF"
// when the binlog is not active).
func (s *ClusterDbReplicationService) loadMysqlStatus(node model.ClusterNode, dbType, instance string) (string, string, error) {
	var status struct {
		File     string `json:"File"`
		Position string `json:"Position"`
	}
	body := map[string]interface{}{"type": dbType, "name": instance}
	if err := s.callNode(node, http.MethodPost, "/api/v2/databases/status", body, &status, 30*time.Second); err != nil {
		return "", "", err
	}
	return status.File, status.Position, nil
}

// updateMysqlVariables writes my.cnf parameters and restarts the instance
// (the agent restarts the compose stack after writing).
func (s *ClusterDbReplicationService) updateMysqlVariables(node model.ClusterNode, dbType, instance string, vars map[string]interface{}) error {
	var list []map[string]interface{}
	for k, v := range vars {
		list = append(list, map[string]interface{}{"param": k, "value": v})
	}
	body := map[string]interface{}{"type": dbType, "database": instance, "variables": list}
	return s.callNode(node, http.MethodPost, "/api/v2/databases/variables/update", body, nil, 5*time.Minute)
}

// updateConfByFile writes the whole config file and restarts the instance.
func (s *ClusterDbReplicationService) updateConfByFile(node model.ClusterNode, dbType, instance, content string) error {
	body := map[string]interface{}{"type": dbType, "database": instance, "file": content}
	return s.callNode(node, http.MethodPost, "/api/v2/databases/common/update/conf", body, nil, 5*time.Minute)
}

// checkDatabaseConn asks a node to open a real database connection to the
// given address with the given credentials (connectivity + auth in one).
func (s *ClusterDbReplicationService) checkDatabaseConn(node model.ClusterNode, dbType, address string, port uint, username, password, initialDB, version string) (bool, error) {
	var ok bool
	body := map[string]interface{}{
		"name": "cluster-repl-check", "type": dbType, "from": "remote",
		"version": version, "address": address, "port": port,
		"username": username, "password": password, "initialDB": initialDB,
		"timeout": 30,
	}
	if err := s.callNode(node, http.MethodPost, "/api/v2/databases/db/check", body, &ok, 60*time.Second); err != nil {
		return false, err
	}
	return ok, nil
}

// listMysqlUsers returns "user@host" strings for an instance.
func (s *ClusterDbReplicationService) listMysqlUsers(node model.ClusterNode, instance string) ([]string, error) {
	var users []struct {
		Username string `json:"username"`
		Host     string `json:"host"`
	}
	body := map[string]interface{}{"database": instance}
	if err := s.callNode(node, http.MethodPost, "/api/v2/databases/users/search", body, &users, 30*time.Second); err != nil {
		return nil, err
	}
	var out []string
	for _, u := range users {
		out = append(out, u.Username+"@"+u.Host)
	}
	return out, nil
}

// createMysqlDB creates an empty schema (no user) inside an instance.
func (s *ClusterDbReplicationService) createMysqlDB(node model.ClusterNode, instance, name string) error {
	body := map[string]interface{}{
		"name": name, "from": "local", "database": instance,
		"format": "utf8mb4", "permission": "localhost",
	}
	return s.callNode(node, http.MethodPost, "/api/v2/databases", body, nil, 60*time.Second)
}

// createPgDB creates an empty schema inside a PostgreSQL instance.
func (s *ClusterDbReplicationService) createPgDB(node model.ClusterNode, instance, name string) error {
	body := map[string]interface{}{
		"name": name, "from": "local", "database": instance,
		"format": "UTF8",
	}
	return s.callNode(node, http.MethodPost, "/api/v2/databases/pg", body, nil, 60*time.Second)
}

// triggerBackup starts an async agent backup and returns the client task id
// used to poll it.
func (s *ClusterDbReplicationService) triggerBackup(node model.ClusterNode, backupType, instance, db string, args []string) (string, error) {
	taskID := randomTaskID("cluster-dbrepl-backup")
	body := map[string]interface{}{
		"type": backupType, "name": instance, "detailName": db,
		"args": args, "taskID": taskID,
	}
	if err := s.callNode(node, http.MethodPost, "/api/v2/backups/backup", body, nil, 60*time.Second); err != nil {
		return "", err
	}
	return taskID, nil
}

// pollTask waits for an agent task id to reach Success/Failed.
func (s *ClusterDbReplicationService) pollTask(node model.ClusterNode, taskID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var page agentPage
		body := map[string]interface{}{"taskID": taskID, "page": 1, "pageSize": 1}
		if err := s.callNode(node, http.MethodPost, "/api/v2/logs/tasks/search", body, &page, 30*time.Second); err != nil {
			return err
		}
		if len(page.Items) > 0 {
			var task struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(page.Items[0], &task) == nil {
				switch task.Status {
				case "Success":
					return nil
				case "Failed":
					return fmt.Errorf("agent task %s failed", taskID)
				}
			}
		}
		time.Sleep(snapshotPollInterval)
	}
	return fmt.Errorf("timed out waiting for agent task %s", taskID)
}

// findBackupRecord returns the latest backup record for a task id.
func (s *ClusterDbReplicationService) findBackupRecord(node model.ClusterNode, backupType, instance, db, taskID string) (string, string, error) {
	var page agentPage
	body := map[string]interface{}{"type": backupType, "name": instance, "detailName": db, "page": 1, "pageSize": 5}
	if err := s.callNode(node, http.MethodPost, "/api/v2/backups/record/search", body, &page, 30*time.Second); err != nil {
		return "", "", err
	}
	for _, raw := range page.Items {
		var rec struct {
			FileDir  string `json:"fileDir"`
			FileName string `json:"fileName"`
			Status   string `json:"status"`
			TaskID   string `json:"taskID"`
			Message  string `json:"message"`
		}
		if json.Unmarshal(raw, &rec) != nil || rec.TaskID != taskID {
			continue
		}
		if rec.Status == "Failed" {
			return "", "", fmt.Errorf("backup failed: %s", rec.Message)
		}
		if rec.Status == "Success" && rec.FileName != "" {
			return rec.FileDir, rec.FileName, nil
		}
		return "", "", fmt.Errorf("backup record not successful yet (status %s)", rec.Status)
	}
	return "", "", fmt.Errorf("backup record for task %s not found", taskID)
}

// recoverByUpload restores a dump file already present on the node.
func (s *ClusterDbReplicationService) recoverByUpload(node model.ClusterNode, backupType, instance, db, filePath string) (string, error) {
	taskID := randomTaskID("cluster-dbrepl-recover")
	body := map[string]interface{}{
		"downloadAccountID": 1, "type": backupType, "name": instance,
		"detailName": db, "file": filePath, "taskID": taskID,
	}
	if err := s.callNode(node, http.MethodPost, "/api/v2/backups/recover/byupload", body, nil, 60*time.Second); err != nil {
		return "", err
	}
	return taskID, nil
}

// ---------- snapshot file transfer (streaming, no 200 MB cap) ----------

func (s *ClusterDbReplicationService) downloadSnapshot(node model.ClusterNode, remotePath string) (string, error) {
	if isLocalNode(node) {
		return s.downloadSnapshotLocal(remotePath)
	}
	client, timestamp, hmacToken, md5Token, err := s.sync.signedClient(node, 10*time.Minute)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "cluster-dbrepl-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	apiBase, err := panelAPIBase(node.EntryURL)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	targetURL := apiBase + "/api/v2/files/download"
	doGet := func(token string) error {
		resp, err := client.R().
			SetHeader("1Panel-Timestamp", timestamp).
			SetHeader("1Panel-Token", token).
			SetQueryParam("path", remotePath).
			SetOutput(tmpPath).
			Get(targetURL)
		if err != nil {
			return err
		}
		if resp.StatusCode() == http.StatusUnauthorized {
			return fmt.Errorf("unauthorized")
		}
		if resp.StatusCode() != http.StatusOK {
			return fmt.Errorf("source panel returned HTTP %d while downloading the snapshot", resp.StatusCode())
		}
		return nil
	}
	if err := doGet(hmacToken); err != nil && err.Error() == "unauthorized" {
		if err := doGet(md5Token); err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}
	} else if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := checkDownloadedFile(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func (s *ClusterDbReplicationService) downloadSnapshotLocal(remotePath string) (string, error) {
	tmp, err := os.CreateTemp("", "cluster-dbrepl-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	targetURL := "http://localhost/api/v2/files/download?path=" + url.QueryEscape(remotePath)
	resp, err := localAgentHTTPClient(10 * time.Minute).Get(targetURL)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("local panel returned HTTP %d while downloading the snapshot", resp.StatusCode)
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		raw, _ := io.ReadAll(resp.Body)
		_ = os.Remove(tmpPath)
		var env remoteEnvelope
		if json.Unmarshal(raw, &env) == nil && strings.TrimSpace(env.Message) != "" {
			return "", fmt.Errorf("local panel error: %s", truncateErr(env.Message))
		}
		return "", fmt.Errorf("local panel returned an unreadable download response")
	}
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	written, copyErr := io.CopyN(out, resp.Body, maxSnapshotBytes+1)
	_ = out.Close()
	if copyErr != nil && copyErr != io.EOF {
		_ = os.Remove(tmpPath)
		return "", copyErr
	}
	if written > maxSnapshotBytes {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("snapshot is over the %d-byte transfer limit", maxSnapshotBytes)
	}
	return tmpPath, nil
}

// checkDownloadedFile rejects a JSON error envelope saved as a file and
// enforces the snapshot size guard.
func checkDownloadedFile(tmpPath string) error {
	info, err := os.Stat(tmpPath)
	if err != nil {
		return err
	}
	if info.Size() > maxSnapshotBytes {
		return fmt.Errorf("snapshot is %d bytes, over the %d-byte transfer limit", info.Size(), maxSnapshotBytes)
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	trimmed := strings.TrimSpace(string(head[:n]))
	if strings.HasPrefix(trimmed, "{") {
		var env remoteEnvelope
		if json.Unmarshal([]byte(trimmed), &env) == nil && env.Code != 0 && env.Code != http.StatusOK {
			if strings.TrimSpace(env.Message) != "" {
				return fmt.Errorf("source panel error: %s", truncateErr(env.Message))
			}
			return fmt.Errorf("source panel error code %d", env.Code)
		}
	}
	return nil
}

func (s *ClusterDbReplicationService) uploadSnapshot(node model.ClusterNode, dir, fileName, tmpPath string) error {
	if isLocalNode(node) {
		return s.sync.uploadLocalFile(context.Background(), dir, fileName, tmpPath, true)
	}
	return s.sync.uploadRemoteFile(context.Background(), node, dir, fileName, tmpPath, true, nil)
}

// ---------- CRUD ----------

func (s *ClusterDbReplicationService) List() ([]dto.ClusterDbReplicationInfo, error) {
	reps, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]dto.ClusterDbReplicationInfo, 0, len(reps))
	for _, rep := range reps {
		out = append(out, replInfo(rep))
	}
	return out, nil
}

func (s *ClusterDbReplicationService) Get(id uint) (dto.ClusterDbReplicationInfo, error) {
	rep, err := s.getRep(id)
	if err != nil {
		return dto.ClusterDbReplicationInfo{}, err
	}
	return replInfo(rep), nil
}

func (s *ClusterDbReplicationService) Create(req dto.ClusterDbReplicationCreate) (uint, error) {
	if !dto.ValidClusterDbType(req.DBType) {
		return 0, buserr.New("ErrInvalidParams")
	}
	if req.MasterNodeID == req.ReplicaNodeID {
		return 0, fmt.Errorf("master and replica must be different nodes")
	}
	if !validDBIdentifier(req.Database) || !validDBIdentifier(req.ReplicaDatabase) {
		return 0, fmt.Errorf("invalid database name")
	}
	if !validDBIdentifier(req.ReplUsername) {
		return 0, fmt.Errorf("invalid replication username")
	}
	master, err := s.node(req.MasterNodeID)
	if err != nil {
		return 0, err
	}
	replica, err := s.node(req.ReplicaNodeID)
	if err != nil {
		return 0, err
	}
	rep := &model.ClusterDbReplication{
		Name: req.Name, DBType: req.DBType,
		MasterNodeID: req.MasterNodeID, MasterName: master.Name,
		MasterInstance: req.MasterInstance, MasterHost: req.MasterHost,
		MasterPort: req.MasterPort, Database: req.Database,
		ReplicaNodeID: req.ReplicaNodeID, ReplicaName: replica.Name,
		ReplicaInstance: req.ReplicaInstance, ReplicaDatabase: req.ReplicaDatabase,
		ReplUsername: req.ReplUsername, Status: model.ClusterDbReplStatusDraft,
		SnapshotStatus: "idle",
	}
	if err := s.repo.Create(rep); err != nil {
		return 0, err
	}
	cipher, err := encryptReplPassword(req.ReplPassword, rep.ID)
	if err != nil {
		return 0, err
	}
	if err := s.repo.Update(rep.ID, map[string]interface{}{"repl_password_cipher": cipher}); err != nil {
		return 0, err
	}
	if err := s.storeManualSteps(rep.ID, s.accountSteps(*rep)); err != nil {
		return 0, err
	}
	return rep.ID, nil
}

func (s *ClusterDbReplicationService) Update(req dto.ClusterDbReplicationUpdate) error {
	rep, err := s.getRep(req.ID)
	if err != nil {
		return err
	}
	vars := map[string]interface{}{}
	if req.Name != "" {
		vars["name"] = req.Name
	}
	if req.ReplUsername != "" {
		if !validDBIdentifier(req.ReplUsername) {
			return fmt.Errorf("invalid replication username")
		}
		vars["repl_username"] = req.ReplUsername
	}
	if len(vars) > 0 {
		if err := s.repo.Update(rep.ID, vars); err != nil {
			return err
		}
	}
	if req.ReplPassword != "" {
		cipher, err := encryptReplPassword(req.ReplPassword, rep.ID)
		if err != nil {
			return err
		}
		if err := s.repo.Update(rep.ID, map[string]interface{}{"repl_password_cipher": cipher}); err != nil {
			return err
		}
	}
	return nil
}

func (s *ClusterDbReplicationService) Delete(id uint) error {
	if _, err := s.getRep(id); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

func (s *ClusterDbReplicationService) ListInstances(req dto.ClusterDbInstancesReq) ([]dto.ClusterDbInstance, error) {
	if !dto.ValidClusterDbType(req.DBType) {
		return nil, buserr.New("ErrInvalidParams")
	}
	node, err := s.node(req.NodeID)
	if err != nil {
		return nil, err
	}
	return s.listInstances(node, req.DBType)
}

func (s *ClusterDbReplicationService) ListSchemas(req dto.ClusterDbSchemasReq) ([]dto.ClusterDbSchema, error) {
	if !dto.ValidClusterDbType(req.DBType) {
		return nil, buserr.New("ErrInvalidParams")
	}
	node, err := s.node(req.NodeID)
	if err != nil {
		return nil, err
	}
	names, err := s.listSchemas(node, req.DBType, req.Instance)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ClusterDbSchema, 0, len(names))
	for _, n := range names {
		out = append(out, dto.ClusterDbSchema{Name: n})
	}
	return out, nil
}

func (s *ClusterDbReplicationService) ListFailovers(id uint) ([]dto.ClusterDbFailoverInfo, error) {
	if _, err := s.getRep(id); err != nil {
		return nil, err
	}
	list, err := s.repo.ListFailovers(id)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ClusterDbFailoverInfo, 0, len(list))
	for _, f := range list {
		out = append(out, dto.ClusterDbFailoverInfo{
			ID: f.ID, ReplicationID: f.ReplicationID,
			FromNodeID: f.FromNodeID, FromName: f.FromName,
			ToNodeID: f.ToNodeID, ToName: f.ToName,
			Reason: f.Reason, Repointed: f.Repointed,
			CreatedAt: f.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return out, nil
}

// ---------- manual steps ----------

func (s *ClusterDbReplicationService) storeManualSteps(id uint, steps []dto.ClusterDbManualStep) error {
	raw, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	return s.repo.Update(id, map[string]interface{}{"manual_steps": string(raw)})
}

func (s *ClusterDbReplicationService) loadManualSteps(rep model.ClusterDbReplication) []dto.ClusterDbManualStep {
	if strings.TrimSpace(rep.ManualSteps) == "" {
		return nil
	}
	var steps []dto.ClusterDbManualStep
	if err := json.Unmarshal([]byte(rep.ManualSteps), &steps); err != nil {
		return nil
	}
	return steps
}

func (s *ClusterDbReplicationService) markManualStepsDone(rep model.ClusterDbReplication, keys []string) ([]dto.ClusterDbManualStep, error) {
	steps := s.loadManualSteps(rep)
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	for i := range steps {
		if want[steps[i].Key] {
			steps[i].Done = true
		}
	}
	if err := s.storeManualSteps(rep.ID, steps); err != nil {
		return nil, err
	}
	return steps, nil
}

// accountSteps is the phase-1 manual work: create the replication account
// on the master. For PostgreSQL the slot + publication are also manual
// and must be done BEFORE the snapshot (the slot pins the WAL position
// the subscription later resumes from).
func (s *ClusterDbReplicationService) accountSteps(rep model.ClusterDbReplication) []dto.ClusterDbManualStep {
	password, err := decryptReplPassword(rep)
	if err != nil {
		password = ""
	}
	if isMysqlType(rep.DBType) {
		sql, err := mysqlReplUserSQL(rep.ReplUsername, "%", password)
		if err != nil {
			sql = "-- 無法產生 SQL：" + err.Error()
		}
		return []dto.ClusterDbManualStep{{
			Key: "create-repl-user", Title: "在主庫建立複製帳號（主庫節點手動執行）", SQL: sql,
		}}
	}
	roleSQL, _ := pgReplRoleSQL(rep.ReplUsername, password)
	pubSQL, _ := pgPublicationSQL(publicationName(rep), rep.Database)
	slotSQL, _ := pgSlotSQL(slotName(rep), rep.Database)
	return []dto.ClusterDbManualStep{
		{Key: "create-repl-user", Title: "在主庫建立複製角色（主庫節點手動執行）", SQL: roleSQL},
		{Key: "create-publication", Title: "在主庫建立 Publication（快照前必須完成）", SQL: pubSQL},
		{Key: "create-slot", Title: "在主庫建立邏輯複製槽（快照前必須完成，鎖定 WAL 位點）", SQL: slotSQL},
	}
}

// startSteps is the phase-3 manual work: point the replica at the master
// and start replication after the snapshot has been restored.
func (s *ClusterDbReplicationService) startSteps(rep model.ClusterDbReplication) ([]dto.ClusterDbManualStep, error) {
	password, err := decryptReplPassword(rep)
	if err != nil {
		return nil, err
	}
	if isMysqlType(rep.DBType) {
		sql, err := mysqlChangeSourceSQL(rep.MasterHost, rep.MasterPort, rep.ReplUsername, password)
		if err != nil {
			return nil, err
		}
		return []dto.ClusterDbManualStep{{
			Key: "start-replication", Title: "在從庫指向主庫並啟動複製（從庫節點手動執行）", SQL: sql,
		}}, nil
	}
	sql, err := pgSubscriptionSQL(subscriptionName(rep), rep.MasterHost, rep.MasterPort, rep.Database, rep.ReplUsername, password, publicationName(rep), slotName(rep))
	if err != nil {
		return nil, err
	}
	return []dto.ClusterDbManualStep{{
		Key: "start-replication", Title: "在從庫建立 Subscription（從庫節點手動執行）", SQL: sql,
	}}, nil
}

// promoteSteps is the manual work of promoting a replica: stop applying
// replication and detach from the old master.
func (s *ClusterDbReplicationService) promoteSteps(rep model.ClusterDbReplication) []dto.ClusterDbManualStep {
	if isMysqlType(rep.DBType) {
		return []dto.ClusterDbManualStep{{
			Key: "stop-replica", Title: "在從庫停止複製並解除主從關係（從庫節點手動執行）",
			SQL: "STOP REPLICA;\nRESET REPLICA ALL;",
		}}
	}
	return []dto.ClusterDbManualStep{{
		Key: "stop-replica", Title: "在從庫刪除 Subscription（從庫節點手動執行）",
		SQL: fmt.Sprintf("DROP SUBSCRIPTION %s;", subscriptionName(rep)),
	}}
}

func (s *ClusterDbReplicationService) ManualSteps(id uint) ([]dto.ClusterDbManualStep, error) {
	rep, err := s.getRep(id)
	if err != nil {
		return nil, err
	}
	return s.loadManualSteps(rep), nil
}

// MarkManualStepsDone ticks manual steps off (e.g. the PostgreSQL slot +
// publication steps that must be done before the snapshot). The panel
// trusts the user's confirmation: it cannot verify these SQL statements
// ran.
func (s *ClusterDbReplicationService) MarkManualStepsDone(id uint, keys []string) error {
	rep, err := s.getRep(id)
	if err != nil {
		return err
	}
	_, err = s.markManualStepsDone(rep, keys)
	return err
}

// PromoteSteps returns the manual detach SQL for failover. It is not
// stored until Promote runs (the stored steps keep describing the
// replication setup until then).
func (s *ClusterDbReplicationService) PromoteSteps(id uint) ([]dto.ClusterDbManualStep, error) {
	rep, err := s.getRep(id)
	if err != nil {
		return nil, err
	}
	return s.promoteSteps(rep), nil
}

// ---------- preflight ----------

func checkPass(name, detail string) dto.ClusterDbPreflightCheck {
	return dto.ClusterDbPreflightCheck{Name: name, Status: "pass", Detail: detail}
}

func checkFail(name, detail string) dto.ClusterDbPreflightCheck {
	return dto.ClusterDbPreflightCheck{Name: name, Status: "fail", Detail: detail}
}

func checkWarn(name, detail string) dto.ClusterDbPreflightCheck {
	return dto.ClusterDbPreflightCheck{Name: name, Status: "warn", Detail: detail}
}

func checkManual(name, detail string) dto.ClusterDbPreflightCheck {
	return dto.ClusterDbPreflightCheck{Name: name, Status: "manual", Detail: detail}
}

// Preflight verifies every automatable precondition and records the
// result on the topology. It never changes anything on either node.
func (s *ClusterDbReplicationService) Preflight(id uint) ([]dto.ClusterDbPreflightCheck, error) {
	rep, err := s.getRep(id)
	if err != nil {
		return nil, err
	}
	checks := []dto.ClusterDbPreflightCheck{}
	master, err := s.node(rep.MasterNodeID)
	if err != nil {
		return nil, err
	}
	replica, err := s.node(rep.ReplicaNodeID)
	if err != nil {
		return nil, err
	}

	// 1. Nodes reachable.
	if rep.MasterNodeID == 0 {
		checks = append(checks, checkPass("主庫節點在線", "本機節點"))
	} else if master.Status == model.ClusterNodeStatusOnline {
		checks = append(checks, checkPass("主庫節點在線", master.Name))
	} else {
		checks = append(checks, checkFail("主庫節點在線", "節點 "+master.Name+" 目前離線"))
	}
	if rep.ReplicaNodeID == 0 {
		checks = append(checks, checkPass("從庫節點在線", "本機節點"))
	} else if replica.Status == model.ClusterNodeStatusOnline {
		checks = append(checks, checkPass("從庫節點在線", replica.Name))
	} else {
		checks = append(checks, checkFail("從庫節點在線", "節點 "+replica.Name+" 目前離線"))
	}

	// 2. Instances exist on both nodes.
	masterInstances, mErr := s.listInstances(master, rep.DBType)
	replicaInstances, rErr := s.listInstances(replica, rep.DBType)
	var masterInst, replicaInst *dto.ClusterDbInstance
	if mErr != nil {
		checks = append(checks, checkFail("主庫實例存在", mErr.Error()))
	} else {
		for i := range masterInstances {
			if masterInstances[i].Name == rep.MasterInstance {
				masterInst = &masterInstances[i]
				break
			}
		}
		if masterInst == nil {
			checks = append(checks, checkFail("主庫實例存在", "主庫節點上沒有名為 "+rep.MasterInstance+" 的實例"))
		} else {
			checks = append(checks, checkPass("主庫實例存在", masterInst.Name+"（版本 "+masterInst.Version+"）"))
		}
	}
	if rErr != nil {
		checks = append(checks, checkFail("從庫實例存在", rErr.Error()))
	} else {
		for i := range replicaInstances {
			if replicaInstances[i].Name == rep.ReplicaInstance {
				replicaInst = &replicaInstances[i]
				break
			}
		}
		if replicaInst == nil {
			checks = append(checks, checkFail("從庫實例存在", "從庫節點上沒有名為 "+rep.ReplicaInstance+" 的實例"))
		} else {
			checks = append(checks, checkPass("從庫實例存在", replicaInst.Name+"（版本 "+replicaInst.Version+"）"))
		}
	}

	// 3. Version compatibility.
	if masterInst != nil && replicaInst != nil {
		if isPgType(rep.DBType) {
			if parseMajorVersion(replicaInst.Version) < parseMajorVersion(masterInst.Version) {
				checks = append(checks, checkFail("版本相容", fmt.Sprintf("PostgreSQL 從庫版本（%s）低於主庫（%s），邏輯複製要求從庫版本不低於主庫", replicaInst.Version, masterInst.Version)))
			} else {
				checks = append(checks, checkPass("版本相容", "主庫 "+masterInst.Version+"，從庫 "+replicaInst.Version))
			}
		} else {
			if parseMajorVersion(masterInst.Version) != parseMajorVersion(replicaInst.Version) {
				checks = append(checks, checkWarn("版本相容", fmt.Sprintf("主從大版本不同（主庫 %s，從庫 %s），建議同大版本", masterInst.Version, replicaInst.Version)))
			} else {
				checks = append(checks, checkPass("版本相容", "主庫 "+masterInst.Version+"，從庫 "+replicaInst.Version))
			}
		}
	}

	// 4. Master database exists; replica database state.
	if masterInst != nil {
		schemas, err := s.listSchemas(master, rep.DBType, rep.MasterInstance)
		if err != nil {
			checks = append(checks, checkFail("主庫存在", err.Error()))
		} else if containsStr(schemas, rep.Database) {
			checks = append(checks, checkPass("主庫存在", rep.Database))
		} else {
			checks = append(checks, checkFail("主庫存在", "主庫實例上沒有名為 "+rep.Database+" 的庫"))
		}
	}
	if replicaInst != nil {
		schemas, err := s.listSchemas(replica, rep.DBType, rep.ReplicaInstance)
		if err != nil {
			checks = append(checks, checkWarn("從庫狀態", "無法列出從庫實例的庫："+err.Error()))
		} else if containsStr(schemas, rep.ReplicaDatabase) {
			checks = append(checks, checkWarn("從庫狀態", "從庫上已有同名庫 "+rep.ReplicaDatabase+"，快照恢復會覆寫其中資料"))
		} else {
			checks = append(checks, checkPass("從庫狀態", "從庫上無同名庫，快照會自動建立"))
		}
	}

	// 5. Master config: read the config file (the reliable source; the
	// variables API drops replication variables from its DTO).
	if masterInst != nil {
		content, err := s.loadConf(master, rep.DBType, rep.MasterInstance)
		if err != nil {
			checks = append(checks, checkFail("主庫複製設定", "無法讀取設定檔："+err.Error()))
		} else if isMysqlType(rep.DBType) {
			params := parseMyCnf(content)
			serverID := params["server-id"]
			if serverID == "" || serverID == "0" {
				checks = append(checks, checkWarn("主庫複製設定", "未設定 server-id，搭建時會自動寫入並重啟實例"))
			} else {
				checks = append(checks, checkPass("主庫複製設定", "server-id="+serverID))
			}
			binlog := params["log-bin"]
			gtid := strings.ToUpper(params["gtid_mode"])
			if binlog == "" {
				checks = append(checks, checkWarn("主庫複製設定", "未開啟 binlog，搭建時會自動寫入並重啟實例"))
			} else if gtid != "ON" {
				// Enabling GTID on a server whose binlog already holds
				// anonymous transactions can fail startup; the configure
				// step refuses this case and asks for a manual purge.
				gtidShown := params["gtid_mode"]
				if gtidShown == "" {
					gtidShown = "未設定"
				}
				checks = append(checks, checkWarn("主庫複製設定", "binlog 已開但 gtid_mode 未開（目前 "+gtidShown+"），開啟 GTID 前需先清理舊 binlog，見手動步驟"))
			} else if strings.ToUpper(params["binlog_format"]) != "ROW" {
				checks = append(checks, checkWarn("主庫複製設定", "binlog_format 目前是 "+params["binlog_format"]+"，搭建時會改為 ROW 並重啟實例"))
			} else {
				checks = append(checks, checkPass("主庫複製設定", "binlog + GTID + ROW 已開啟"))
			}
		} else {
			params := parsePgConf(content)
			if strings.ToLower(params["wal_level"]) != "logical" {
				checks = append(checks, checkWarn("主庫複製設定", "wal_level 目前是 "+params["wal_level"]+"，搭建時會改為 logical 並重啟實例"))
			} else {
				checks = append(checks, checkPass("主庫複製設定", "wal_level=logical 已開啟"))
			}
		}
		// 6. Master binlog active (MySQL only; PG has no equivalent read).
		if isMysqlType(rep.DBType) {
			file, pos, err := s.loadMysqlStatus(master, rep.DBType, rep.MasterInstance)
			if err != nil {
				checks = append(checks, checkWarn("主庫 binlog 狀態", "無法讀取："+err.Error()))
			} else if file == "" || file == "OFF" {
				checks = append(checks, checkWarn("主庫 binlog 狀態", "binlog 目前未啟用，搭建完成重啟後會啟用"))
			} else {
				checks = append(checks, checkPass("主庫 binlog 狀態", "binlog "+file+" @ "+pos))
			}
		}
	}

	// 7. Replica server-id must differ from the master's (MySQL only).
	if isMysqlType(rep.DBType) && masterInst != nil && replicaInst != nil {
		mContent, mErr := s.loadConf(master, rep.DBType, rep.MasterInstance)
		rContent, rErr := s.loadConf(replica, rep.DBType, rep.ReplicaInstance)
		if mErr == nil && rErr == nil {
			mID := parseMyCnf(mContent)["server-id"]
			rID := parseMyCnf(rContent)["server-id"]
			if mID != "" && mID != "0" && rID != "" && rID != "0" && mID == rID {
				checks = append(checks, checkFail("server-id 不重複", "主從 server-id 相同（"+mID+"），複製會拒絕連線，搭建時會自動修正從庫"))
			} else {
				checks = append(checks, checkPass("server-id 不重複", "主從 server-id 不同或尚未設定"))
			}
		}
	}

	// 8. Replication account exists (MySQL user list; PG has no list API
	//    for roles, so it stays manual).
	if isMysqlType(rep.DBType) && masterInst != nil {
		users, err := s.listMysqlUsers(master, rep.MasterInstance)
		if err != nil {
			checks = append(checks, checkWarn("複製帳號存在", "無法列出主庫用戶："+err.Error()))
		} else if containsStr(users, rep.ReplUsername+"@%") {
			checks = append(checks, checkPass("複製帳號存在", rep.ReplUsername+"@% 已建立"))
		} else {
			checks = append(checks, checkManual("複製帳號存在", "主庫上沒有 "+rep.ReplUsername+"@%，請按手動步驟建立後再驗證連通性"))
		}
	} else if isPgType(rep.DBType) {
		checks = append(checks, checkManual("複製帳號存在", "面板沒有列出 PG 角色的接口，請按手動步驟建立角色後再驗證連通性"))
	}

	// 9. Connectivity from the replica node to the master database, using
	//    the replication credentials. Only meaningful once the account
	//    exists; otherwise report as manual.
	password, _ := decryptReplPassword(rep)
	masterVersion := ""
	if masterInst != nil {
		masterVersion = masterInst.Version
	}
	if masterVersion == "" {
		if isPgType(rep.DBType) {
			masterVersion = "16"
		} else {
			masterVersion = "8.0"
		}
	}
	if rep.MasterHost != "" && rep.MasterPort != 0 {
		ok, err := s.checkDatabaseConn(replica, rep.DBType, rep.MasterHost, rep.MasterPort, rep.ReplUsername, password, rep.Database, masterVersion)
		if err != nil {
			checks = append(checks, checkWarn("從庫連通主庫", "連通性檢查呼叫失敗："+err.Error()))
		} else if ok {
			checks = append(checks, checkPass("從庫連通主庫", fmt.Sprintf("從庫節點可連 %s:%d 並通過複製帳號認證", rep.MasterHost, rep.MasterPort)))
		} else {
			checks = append(checks, checkManual("從庫連通主庫", fmt.Sprintf("從庫節點連不到 %s:%d 或複製帳號認證失敗：先確認帳號已建立，再確認防火牆/安全組放行 %d 埠；若埠無法互通，需按手動步驟建 SSH 隧道", rep.MasterHost, rep.MasterPort, rep.MasterPort)))
		}
	}

	// 10. Honest unknowns: replica lag and disk space have no API.
	checks = append(checks,
		checkManual("複製延遲監控", "面板沒有讀取從庫複製狀態的接口，延遲顯示為 —；請以 SHOW REPLICA STATUS / pg_stat_subscription 核對"),
		checkManual("從庫磁碟空間", "面板沒有讀取節點磁碟的接口，請確認從庫磁碟空間大於主庫資料量"),
	)

	raw, _ := json.Marshal(checks)
	status := model.ClusterDbReplStatusDraft
	failed := false
	for _, c := range checks {
		if c.Status == "fail" {
			failed = true
		}
	}
	if !failed {
		status = model.ClusterDbReplStatusReady
	}
	_ = s.repo.Update(rep.ID, map[string]interface{}{
		"preflight": raw, "status": status, "setup_step": "preflight",
	})
	return checks, nil
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// ---------- configure master ----------

// ConfigureMaster writes the replication startup parameters and restarts
// the instances (the agent restarts the compose stack after writing).
// The caller must confirm the restart in the UI: this call restarts the
// master database instance and the replica instance.
func (s *ClusterDbReplicationService) ConfigureMaster(id uint) error {
	rep, err := s.getRep(id)
	if err != nil {
		return err
	}
	master, err := s.node(rep.MasterNodeID)
	if err != nil {
		return err
	}
	replica, err := s.node(rep.ReplicaNodeID)
	if err != nil {
		return err
	}
	_ = s.repo.Update(rep.ID, map[string]interface{}{"status": model.ClusterDbReplStatusConfiguring, "last_error": ""})

	masterServerID, replicaServerID := 1000+int(rep.MasterNodeID), 2000+int(rep.ReplicaNodeID)
	if isMysqlType(rep.DBType) {
		content, err := s.loadConf(master, rep.DBType, rep.MasterInstance)
		if err != nil {
			return s.failConfigure(rep, err)
		}
		params := parseMyCnf(content)
		if v := params["server-id"]; v != "" && v != "0" {
			if n, err := strconv.Atoi(v); err == nil {
				masterServerID = n
			}
		}
		binlogWasOn := params["log-bin"] != ""
		gtid := strings.ToUpper(params["gtid_mode"])
		if binlogWasOn && gtid != "ON" {
			// Anonymous transactions may already sit in the binlog;
			// forcing gtid_mode=ON in my.cnf can fail startup. Refuse
			// and ask for a manual purge instead of risking the master.
			return s.failConfigure(rep, fmt.Errorf("主庫 binlog 已開但 gtid_mode 未開（目前 %s）：請先手動執行 PURGE BINARY LOGS 清理舊 binlog 再重試", params["gtid_mode"]))
		}
		vars := map[string]interface{}{
			"server-id":                masterServerID,
			"log-bin":                  "mysql-bin",
			"binlog_format":            "ROW",
			"gtid_mode":                "ON",
			"enforce_gtid_consistency": "ON",
		}
		if err := s.updateMysqlVariables(master, rep.DBType, rep.MasterInstance, vars); err != nil {
			return s.failConfigure(rep, err)
		}
		// Replica: a distinct server-id (keep the existing one if set).
		rContent, err := s.loadConf(replica, rep.DBType, rep.ReplicaInstance)
		if err != nil {
			return s.failConfigure(rep, err)
		}
		rParams := parseMyCnf(rContent)
		if v := rParams["server-id"]; v != "" && v != "0" {
			if n, err := strconv.Atoi(v); err == nil {
				replicaServerID = n
			}
		}
		if replicaServerID == masterServerID {
			replicaServerID = masterServerID + 1
		}
		if err := s.updateMysqlVariables(replica, rep.DBType, rep.ReplicaInstance, map[string]interface{}{"server-id": replicaServerID}); err != nil {
			return s.failConfigure(rep, err)
		}
	} else {
		content, err := s.loadConf(master, rep.DBType, rep.MasterInstance)
		if err != nil {
			return s.failConfigure(rep, err)
		}
		content = setPgConfParam(content, "wal_level", "'logical'")
		content = setPgConfParam(content, "max_replication_slots", "10")
		content = setPgConfParam(content, "max_wal_senders", "10")
		if err := s.updateConfByFile(master, rep.DBType, rep.MasterInstance, content); err != nil {
			return s.failConfigure(rep, err)
		}
	}
	if err := s.repo.Update(rep.ID, map[string]interface{}{
		"master_server_id": masterServerID, "replica_server_id": replicaServerID,
		"setup_step": "master_config", "status": model.ClusterDbReplStatusReady,
	}); err != nil {
		return err
	}
	return nil
}

func (s *ClusterDbReplicationService) failConfigure(rep model.ClusterDbReplication, err error) error {
	_ = s.repo.Update(rep.ID, map[string]interface{}{
		"status": model.ClusterDbReplStatusError, "last_error": err.Error(),
	})
	return err
}

// ---------- snapshot ----------

// StartSnapshot begins the initial snapshot in the background: backup on
// the master, transfer to the replica, restore, then flip the replica to
// read-only. It returns immediately; progress is on the topology row and
// polled through Get. For PostgreSQL the manual slot + publication steps
// must be confirmed first (the slot pins the WAL position).
func (s *ClusterDbReplicationService) StartSnapshot(id uint) error {
	rep, err := s.getRep(id)
	if err != nil {
		return err
	}
	if isPgType(rep.DBType) {
		for _, step := range s.loadManualSteps(rep) {
			if (step.Key == "create-publication" || step.Key == "create-slot") && !step.Done {
				return fmt.Errorf("PostgreSQL 需先完成手動步驟「%s」，再開始快照", step.Title)
			}
		}
	}
	if rep.SnapshotStatus == "running" {
		return fmt.Errorf("snapshot already running")
	}
	_ = s.repo.Update(rep.ID, map[string]interface{}{
		"status":          model.ClusterDbReplStatusSnapshot,
		"snapshot_status": "running", "snapshot_detail": "開始快照…",
		"last_error": "",
	})
	go s.runSnapshot(rep.ID)
	return nil
}

func (s *ClusterDbReplicationService) setSnapshot(id uint, status, detail string) {
	_ = s.repo.Update(id, map[string]interface{}{
		"snapshot_status": status, "snapshot_detail": detail,
	})
}

func (s *ClusterDbReplicationService) runSnapshot(id uint) {
	rep, err := s.getRep(id)
	if err != nil {
		return
	}
	fail := func(err error) {
		s.setSnapshot(id, "failed", err.Error())
		_ = s.repo.Update(id, map[string]interface{}{
			"status": model.ClusterDbReplStatusError, "last_error": err.Error(),
		})
	}
	master, err := s.node(rep.MasterNodeID)
	if err != nil {
		fail(err)
		return
	}
	replica, err := s.node(rep.ReplicaNodeID)
	if err != nil {
		fail(err)
		return
	}

	// 1. Replica database must exist before restore (the agent recover
	//    looks the database up by name). Create an empty one if missing;
	//    if creation fails, the user can create it in the replica panel's
	//    database page and retry.
	s.setSnapshot(id, "running", "確認從庫資料庫…")
	schemas, err := s.listSchemas(replica, rep.DBType, rep.ReplicaInstance)
	if err != nil {
		fail(fmt.Errorf("無法列出從庫實例的庫：%w", err))
		return
	}
	if !containsStr(schemas, rep.ReplicaDatabase) {
		var cErr error
		if isMysqlType(rep.DBType) {
			cErr = s.createMysqlDB(replica, rep.ReplicaInstance, rep.ReplicaDatabase)
		} else {
			cErr = s.createPgDB(replica, rep.ReplicaInstance, rep.ReplicaDatabase)
		}
		if cErr != nil {
			fail(fmt.Errorf("從庫上沒有 %s，且自動建立失敗（%v）；請先在從庫面板的資料庫頁手動建立該庫再重試", rep.ReplicaDatabase, cErr))
			return
		}
	}

	// 2. Backup on the master. MySQL: consistent InnoDB snapshot with the
	//    GTID position embedded (--set-gtid-purged=ON), so the replica
	//    resumes exactly where the dump was taken.
	s.setSnapshot(id, "running", "在主庫備份…")
	backupType := rep.DBType
	var args []string
	if isMysqlType(rep.DBType) {
		args = []string{"--single-transaction", "--master-data=2", "--set-gtid-purged=ON"}
	}
	backupTaskID, err := s.triggerBackup(master, backupType, rep.MasterInstance, rep.Database, args)
	if err != nil {
		fail(fmt.Errorf("主庫備份啟動失敗：%w", err))
		return
	}
	if err := s.pollTask(master, backupTaskID, snapshotPollTimeout); err != nil {
		detail := "主庫備份失敗：" + err.Error()
		if _, _, rErr := s.findBackupRecord(master, backupType, rep.MasterInstance, rep.Database, backupTaskID); rErr != nil {
			detail = "主庫備份失敗：" + rErr.Error()
		}
		fail(fmt.Errorf("%s", detail))
		return
	}
	fileDir, fileName, err := s.findBackupRecord(master, backupType, rep.MasterInstance, rep.Database, backupTaskID)
	if err != nil {
		fail(err)
		return
	}
	// The record stores a path relative to the node's backup dir; resolve
	// the absolute base from the node itself.
	var localDir struct {
		Dir string `json:"dir"`
	}
	baseDir := "/opt/1panel/backup"
	if err := s.callNode(master, http.MethodGet, "/api/v2/backups/local", nil, &localDir, 30*time.Second); err == nil && localDir.Dir != "" {
		baseDir = localDir.Dir
	}
	remotePath := strings.TrimSuffix(baseDir, "/") + "/" + strings.TrimPrefix(fileDir, "/") + "/" + fileName

	// 3. Transfer the dump master -> replica (streaming).
	s.setSnapshot(id, "running", "傳輸快照到從庫…")
	tmpPath, err := s.downloadSnapshot(master, remotePath)
	if err != nil {
		fail(fmt.Errorf("下載快照失敗：%w", err))
		return
	}
	defer os.Remove(tmpPath)
	replicaTmpDir := "/tmp"
	if err := s.uploadSnapshot(replica, replicaTmpDir, fileName, tmpPath); err != nil {
		fail(fmt.Errorf("上傳快照到從庫失敗：%w", err))
		return
	}
	replicaFile := replicaTmpDir + "/" + fileName

	// 4. Restore on the replica.
	s.setSnapshot(id, "running", "在從庫恢復快照…")
	recoverTaskID, err := s.recoverByUpload(replica, backupType, rep.ReplicaInstance, rep.ReplicaDatabase, replicaFile)
	if err != nil {
		fail(fmt.Errorf("從庫恢復啟動失敗：%w", err))
		return
	}
	if err := s.pollTask(replica, recoverTaskID, snapshotPollTimeout); err != nil {
		fail(fmt.Errorf("從庫恢復失敗：%w", err))
		return
	}

	// 5. Replica read-only, so nobody writes to it by accident.
	s.setSnapshot(id, "running", "設定從庫唯讀…")
	if isMysqlType(rep.DBType) {
		if err := s.updateMysqlVariables(replica, rep.DBType, rep.ReplicaInstance, map[string]interface{}{"read_only": "ON"}); err != nil {
			fail(fmt.Errorf("設定從庫唯讀失敗：%w", err))
			return
		}
	} else {
		content, err := s.loadConf(replica, rep.DBType, rep.ReplicaInstance)
		if err != nil {
			fail(fmt.Errorf("讀取從庫設定檔失敗：%w", err))
			return
		}
		content = setPgConfParam(content, "default_transaction_read_only", "'on'")
		if err := s.updateConfByFile(replica, rep.DBType, rep.ReplicaInstance, content); err != nil {
			fail(fmt.Errorf("設定從庫唯讀失敗：%w", err))
			return
		}
	}

	// 6. Done: the replica now holds the snapshot; replication itself is
	//    started with the manual SQL shown to the user.
	steps, err := s.startSteps(rep)
	if err != nil {
		fail(err)
		return
	}
	_ = s.storeManualSteps(id, steps)
	// Refresh the master binlog position for display.
	if isMysqlType(rep.DBType) {
		if file, pos, err := s.loadMysqlStatus(master, rep.DBType, rep.MasterInstance); err == nil {
			_ = s.repo.Update(id, map[string]interface{}{
				"master_binlog_file": file, "master_binlog_position": pos,
			})
		}
	}
	s.setSnapshot(id, "done", "快照已恢復到從庫；請按手動步驟啟動複製")
	_ = s.repo.Update(id, map[string]interface{}{
		"status": model.ClusterDbReplStatusAwaitingReplication, "setup_step": "snapshot",
	})
}

// ConfirmReplicationStarted records that the user ran the manual start
// SQL and verified replication is running. The panel cannot verify this
// itself (no replica-status API), so the confirmation is the source of
// truth and is timestamped.
func (s *ClusterDbReplicationService) ConfirmReplicationStarted(req dto.ClusterDbConfirmReq) error {
	rep, err := s.getRep(req.ID)
	if err != nil {
		return err
	}
	if rep.Status != model.ClusterDbReplStatusAwaitingReplication && rep.Status != model.ClusterDbReplStatusError {
		return fmt.Errorf("topology is not awaiting replication start (status %s)", rep.Status)
	}
	if _, err := s.markManualStepsDone(rep, req.ManualStepKeys); err != nil {
		return err
	}
	return s.repo.Update(rep.ID, map[string]interface{}{
		"status": model.ClusterDbReplStatusRunning, "setup_step": "manual_pending",
		"last_confirmed_at": time.Now().Format("2006-01-02 15:04:05"),
		"last_error":        "",
	})
}

// RefreshStatus re-reads what the panel can read: master reachability +
// binlog position, replica instance reachability. Replica lag stays
// unknown. A dead master sets FailoverSuggested (detection only).
func (s *ClusterDbReplicationService) RefreshStatus(id uint) (dto.ClusterDbReplicationInfo, error) {
	rep, err := s.getRep(id)
	if err != nil {
		return dto.ClusterDbReplicationInfo{}, err
	}
	master, err := s.node(rep.MasterNodeID)
	if err != nil {
		return dto.ClusterDbReplicationInfo{}, err
	}
	replica, err := s.node(rep.ReplicaNodeID)
	if err != nil {
		return dto.ClusterDbReplicationInfo{}, err
	}
	vars := map[string]interface{}{
		"last_checked_at": time.Now().Format("2006-01-02 15:04:05"),
	}
	if isMysqlType(rep.DBType) {
		file, pos, err := s.loadMysqlStatus(master, rep.DBType, rep.MasterInstance)
		if err != nil {
			vars["failover_suggested"] = true
			vars["last_error"] = "主庫無法讀取狀態：" + err.Error()
		} else {
			vars["master_binlog_file"] = file
			vars["master_binlog_position"] = pos
			vars["failover_suggested"] = false
		}
	} else {
		if _, err := s.listInstances(master, rep.DBType); err != nil {
			vars["failover_suggested"] = true
			vars["last_error"] = "主庫無法連通：" + err.Error()
		} else {
			vars["failover_suggested"] = false
		}
	}
	if _, err := s.listInstances(replica, rep.DBType); err != nil {
		vars["last_error"] = "從庫無法連通：" + err.Error()
	}
	_ = s.repo.Update(rep.ID, vars)
	rep, _ = s.getRep(id)
	return replInfo(rep), nil
}

// ---------- failover ----------

// Promote manually promotes the replica to master: the replica is flipped
// to writable (restart), the user confirms the manual detach SQL, the
// topology swaps master/replica, and every other replica of the same old
// master is re-pointed at the promoted node (record update + fresh manual
// SQL; the re-point SQL itself is still manual). There is deliberately no
// automatic promotion: only a human clicks this button.
func (s *ClusterDbReplicationService) Promote(req dto.ClusterDbPromoteReq) (dto.ClusterDbFailoverInfo, error) {
	rep, err := s.getRep(req.ID)
	if err != nil {
		return dto.ClusterDbFailoverInfo{}, err
	}
	if rep.Status != model.ClusterDbReplStatusRunning && rep.Status != model.ClusterDbReplStatusError {
		return dto.ClusterDbFailoverInfo{}, fmt.Errorf("only a running (or errored) topology can be failed over (status %s)", rep.Status)
	}
	replica, err := s.node(rep.ReplicaNodeID)
	if err != nil {
		return dto.ClusterDbFailoverInfo{}, err
	}
	// 1. Flip the replica to writable. MySQL read_only is a startup
	//    parameter here (set via my.cnf + restart); PostgreSQL uses
	//    default_transaction_read_only in postgresql.conf + restart.
	if isMysqlType(rep.DBType) {
		if err := s.updateMysqlVariables(replica, rep.DBType, rep.ReplicaInstance, map[string]interface{}{"read_only": "OFF"}); err != nil {
			return dto.ClusterDbFailoverInfo{}, fmt.Errorf("解除從庫唯讀失敗：%w", err)
		}
	} else {
		content, err := s.loadConf(replica, rep.DBType, rep.ReplicaInstance)
		if err != nil {
			return dto.ClusterDbFailoverInfo{}, err
		}
		content = setPgConfParam(content, "default_transaction_read_only", "'off'")
		if err := s.updateConfByFile(replica, rep.DBType, rep.ReplicaInstance, content); err != nil {
			return dto.ClusterDbFailoverInfo{}, fmt.Errorf("解除從庫唯讀失敗：%w", err)
		}
	}
	// Record the promote steps (with the confirmed keys ticked) as the
	// topology's final manual steps.
	pSteps := s.promoteSteps(rep)
	for i := range pSteps {
		for _, k := range req.ManualStepKeys {
			if pSteps[i].Key == k {
				pSteps[i].Done = true
			}
		}
	}
	_ = s.storeManualSteps(rep.ID, pSteps)
	// 2. Swap master/replica on this topology.
	oldMasterNodeID, oldMasterName := rep.MasterNodeID, rep.MasterName
	oldMasterInstance := rep.MasterInstance
	oldMasterDB := rep.Database
	newMasterPort := rep.MasterPort
	// The promoted node's database port: read it from the node so the
	// re-pointed replicas get a real port.
	if port, err := s.loadBaseInfo(replica, rep.DBType, rep.ReplicaInstance); err == nil && port != 0 {
		newMasterPort = port
	}
	// The promoted node's reachable host. For a remote replica it is the
	// panel entry host. For a local replica the panel does not know its
	// own public host, so the address is left empty: other replicas are
	// NOT auto-re-pointed in that case (they would get a wrong address),
	// and the topology records that the address must be set by hand.
	newMasterHost := ""
	localPromote := rep.ReplicaNodeID == 0
	if !localPromote {
		if u, err := url.Parse(strings.TrimRight(replica.EntryURL, "/")); err == nil && u.Hostname() != "" {
			newMasterHost = u.Hostname()
		}
	}
	_ = s.repo.Update(rep.ID, map[string]interface{}{
		"master_node_id": rep.ReplicaNodeID, "master_name": rep.ReplicaName,
		"master_instance": rep.ReplicaInstance, "master_host": newMasterHost,
		"master_port": newMasterPort, "database": rep.ReplicaDatabase,
		"replica_node_id": oldMasterNodeID, "replica_name": oldMasterName,
		"replica_instance": oldMasterInstance, "replica_database": oldMasterDB,
		"status": model.ClusterDbReplStatusFailedOver, "setup_step": "",
		"failover_suggested": false,
		"master_server_id":   rep.ReplicaServerID, "replica_server_id": rep.MasterServerID,
	})
	if localPromote {
		_ = s.repo.Update(rep.ID, map[string]interface{}{
			"last_error": "已提升本機為主庫；本機對外地址未知，請手動更新主庫地址後再為其他從庫重新指向",
		})
	}
	// 3. Re-point the other replicas of the same old master at the
	//    promoted node. Their replication SQL is regenerated from the
	//    new master address; starting it is manual per topology.
	//    Skipped when the promoted node is local (no known public host).
	others, _ := s.repo.ListByMaster(oldMasterNodeID, oldMasterInstance, oldMasterDB)
	repointed := 0
	if newMasterHost == "" {
		others = nil
	}
	for _, other := range others {
		if other.ID == rep.ID {
			continue
		}
		other.MasterNodeID = rep.ReplicaNodeID
		other.MasterName = rep.ReplicaName
		other.MasterInstance = rep.ReplicaInstance
		other.MasterHost = newMasterHost
		other.MasterPort = newMasterPort
		other.Database = rep.ReplicaDatabase
		steps, err := s.startSteps(other)
		if err != nil {
			continue
		}
		_ = s.storeManualSteps(other.ID, steps)
		_ = s.repo.Update(other.ID, map[string]interface{}{
			"master_node_id": other.MasterNodeID, "master_name": other.MasterName,
			"master_instance": other.MasterInstance, "master_host": other.MasterHost,
			"master_port": other.MasterPort, "database": other.Database,
			"status": model.ClusterDbReplStatusAwaitingReplication, "setup_step": "snapshot",
		})
		repointed++
	}
	f := &model.ClusterDbFailover{
		ReplicationID: rep.ID,
		FromNodeID:    oldMasterNodeID, FromName: oldMasterName,
		ToNodeID: rep.ReplicaNodeID, ToName: rep.ReplicaName,
		Reason: req.Reason, Repointed: repointed,
	}
	if err := s.repo.CreateFailover(f); err != nil {
		return dto.ClusterDbFailoverInfo{}, err
	}
	return dto.ClusterDbFailoverInfo{
		ID: f.ID, ReplicationID: f.ReplicationID,
		FromNodeID: f.FromNodeID, FromName: f.FromName,
		ToNodeID: f.ToNodeID, ToName: f.ToName,
		Reason: f.Reason, Repointed: f.Repointed,
		CreatedAt: f.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}
