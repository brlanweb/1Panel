package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDbReplTestDB(t *testing.T) func() {
	t.Helper()
	oldDB := global.DB
	oldKey := global.CONF.Base.EncryptKey
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&model.ClusterNode{}, &model.ClusterDbReplication{}, &model.ClusterDbFailover{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	global.DB = db
	global.CONF.Base.EncryptKey = "test-encrypt-key-for-cluster-dbrepl"
	return func() {
		global.DB = oldDB
		global.CONF.Base.EncryptKey = oldKey
	}
}

func TestDbReplParseMyCnf(t *testing.T) {
	content := "[client]\nport=3306\n[mysqld]\nserver-id=101\nlog-bin=mysql-bin\nbinlog_format=ROW\n# gtid_mode=OFF\ngtid_mode=ON\n[mysqldump]\nquick\n"
	params := parseMyCnf(content)
	if params["server-id"] != "101" {
		t.Errorf("server-id = %q, want 101", params["server-id"])
	}
	if params["log-bin"] != "mysql-bin" {
		t.Errorf("log-bin = %q, want mysql-bin", params["log-bin"])
	}
	if params["gtid_mode"] != "ON" {
		t.Errorf("gtid_mode = %q, want ON (commented line must not win)", params["gtid_mode"])
	}
	if _, ok := params["port"]; ok {
		t.Errorf("port from [client] leaked into [mysqld] params")
	}
	if _, ok := params["quick"]; ok {
		t.Errorf("quick from [mysqldump] leaked into [mysqld] params")
	}
}

func TestDbReplSetMyCnfParam(t *testing.T) {
	content := "[mysqld]\nserver-id=101\nlog-bin=mysql-bin\n"
	updated := setMyCnfParam(content, "gtid_mode", "ON")
	params := parseMyCnf(updated)
	if params["gtid_mode"] != "ON" || params["server-id"] != "101" || params["log-bin"] != "mysql-bin" {
		t.Errorf("after set: %v", params)
	}
	updated = setMyCnfParam(updated, "server-id", "102")
	if parseMyCnf(updated)["server-id"] != "102" {
		t.Errorf("server-id not replaced: %q", updated)
	}
	// No [mysqld] group at all: group is appended.
	updated = setMyCnfParam("[client]\nport=3306\n", "server-id", "5")
	if parseMyCnf(updated)["server-id"] != "5" {
		t.Errorf("group not appended: %q", updated)
	}
}

func TestDbReplParsePgConf(t *testing.T) {
	content := "# comment\nwal_level = 'logical' # trailing\nmax_replication_slots = 10\nmax_wal_senders=10\n"
	params := parsePgConf(content)
	if params["wal_level"] != "logical" {
		t.Errorf("wal_level = %q, want logical", params["wal_level"])
	}
	if params["max_replication_slots"] != "10" {
		t.Errorf("max_replication_slots = %q, want 10", params["max_replication_slots"])
	}
	updated := setPgConfParam(content, "wal_level", "'replica'")
	if parsePgConf(updated)["wal_level"] != "replica" {
		t.Errorf("wal_level not replaced: %q", updated)
	}
	updated = setPgConfParam(content, "default_transaction_read_only", "'on'")
	if parsePgConf(updated)["default_transaction_read_only"] != "on" {
		t.Errorf("new param not appended: %q", updated)
	}
}

func TestDbReplParseMajorVersion(t *testing.T) {
	cases := map[string]int{"8.0.36": 8, "16.2": 16, "10.11.7-MariaDB": 10, "5.7.44": 5, "": 0}
	for in, want := range cases {
		if got := parseMajorVersion(in); got != want {
			t.Errorf("parseMajorVersion(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestDbReplSQLBuilders(t *testing.T) {
	sql, err := mysqlReplUserSQL("repl", "%", "s3cret")
	if err != nil {
		t.Fatalf("mysqlReplUserSQL: %v", err)
	}
	if !strings.Contains(sql, "GRANT REPLICATION SLAVE ON *.* TO 'repl'@'%'") {
		t.Errorf("repl user SQL missing grant: %s", sql)
	}
	// A password containing a quote is escaped, not injected.
	sql, err = mysqlReplUserSQL("repl", "%", "p' OR '1'='1")
	if err != nil {
		t.Fatalf("mysqlReplUserSQL: %v", err)
	}
	if !strings.Contains(sql, "p'' OR ''1''=''1") {
		t.Errorf("password not escaped: %s", sql)
	}
	if _, err := mysqlReplUserSQL("bad user", "%", "x"); err == nil {
		t.Errorf("invalid username accepted")
	}
	src, err := mysqlChangeSourceSQL("10.0.0.1", 3306, "repl", "s3cret")
	if err != nil {
		t.Fatalf("mysqlChangeSourceSQL: %v", err)
	}
	if !strings.Contains(src, "SOURCE_AUTO_POSITION=1") || !strings.Contains(src, "START REPLICA") {
		t.Errorf("change source SQL incomplete: %s", src)
	}
	role, err := pgReplRoleSQL("repl", "s3cret")
	if err != nil || !strings.Contains(role, "WITH LOGIN REPLICATION") {
		t.Errorf("pg role SQL = %q, err %v", role, err)
	}
	sub, err := pgSubscriptionSQL("cluster_repl_1", "10.0.0.1", 5432, "appdb", "repl", "s3cret", "cluster_repl_1", "cluster_repl_1")
	if err != nil {
		t.Fatalf("pgSubscriptionSQL: %v", err)
	}
	if !strings.Contains(sub, "copy_data=false") || !strings.Contains(sub, "create_slot=false") {
		t.Errorf("subscription SQL incomplete: %s", sub)
	}
}

// fakeDbAgent serves the agent endpoints the replication service uses.
type fakeDbAgent struct {
	mysqlCnf     string
	pgConf       string
	users        []map[string]interface{}
	binlogFile   string
	binlogPos    string
	varsSeen     []map[string]interface{}
	confSeen     []string
	checkResults map[string]bool
}

func (f *fakeDbAgent) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/databases/db/list/", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, []map[string]interface{}{
			{"id": 1, "database": "mysql", "type": "mysql", "version": "8.0.36"},
		})
	})
	mux.HandleFunc("/api/v2/databases/search", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": 1, "items": []map[string]interface{}{{"name": "appdb"}}})
	})
	mux.HandleFunc("/api/v2/databases/pg/search", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"total": 1, "items": []map[string]interface{}{{"name": "appdb"}}})
	})
	mux.HandleFunc("/api/v2/databases/common/load/file", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Type string `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(body.Type, "postgresql") {
			envelope(w, f.pgConf)
		} else {
			envelope(w, f.mysqlCnf)
		}
	})
	mux.HandleFunc("/api/v2/databases/status", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"File": f.binlogFile, "Position": f.binlogPos})
	})
	mux.HandleFunc("/api/v2/databases/users/search", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, f.users)
	})
	mux.HandleFunc("/api/v2/databases/db/check", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, true)
	})
	mux.HandleFunc("/api/v2/databases/variables/update", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables []map[string]interface{} `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.varsSeen = append(f.varsSeen, body.Variables...)
		envelope(w, nil)
	})
	mux.HandleFunc("/api/v2/databases/common/update/conf", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			File string `json:"file"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.confSeen = append(f.confSeen, body.File)
		envelope(w, nil)
	})
	mux.HandleFunc("/api/v2/databases/common/info", func(w http.ResponseWriter, r *http.Request) {
		envelope(w, map[string]interface{}{"port": 3306})
	})
	return mux
}

func createTestDbRepl(t *testing.T, svc IClusterDbReplicationService, masterID, replicaID uint) uint {
	t.Helper()
	id, err := svc.Create(dto.ClusterDbReplicationCreate{
		Name: "test-repl", DBType: dto.ClusterDbTypeMysql,
		MasterNodeID: masterID, MasterInstance: "mysql",
		MasterHost: "10.0.0.1", MasterPort: 3306, Database: "appdb",
		ReplicaNodeID: replicaID, ReplicaInstance: "mysql",
		ReplicaDatabase: "appdb",
		ReplUsername:    "repl", ReplPassword: "s3cret",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return id
}

func TestDbReplPreflightAllPass(t *testing.T) {
	restore := setupDbReplTestDB(t)
	defer restore()

	masterAgent := &fakeDbAgent{
		mysqlCnf:   "[mysqld]\nserver-id=1000\nlog-bin=mysql-bin\nbinlog_format=ROW\ngtid_mode=ON\nenforce_gtid_consistency=ON\n",
		binlogFile: "mysql-bin.000001", binlogPos: "1234",
		users: []map[string]interface{}{{"username": "repl", "host": "%"}},
	}
	replicaAgent := &fakeDbAgent{
		mysqlCnf: "[mysqld]\nserver-id=2001\n",
	}
	masterSrv := httptest.NewServer(masterAgent.handler())
	defer masterSrv.Close()
	replicaSrv := httptest.NewServer(replicaAgent.handler())
	defer replicaSrv.Close()

	masterNode := insertTestNode(t, "master", masterSrv.URL, model.ClusterNodeStatusOnline)
	replicaNode := insertTestNode(t, "replica", replicaSrv.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterDbReplicationService()
	id := createTestDbRepl(t, svc, masterNode.ID, replicaNode.ID)
	checks, err := svc.Preflight(id)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	for _, c := range checks {
		if c.Status == "fail" {
			t.Errorf("preflight check %q failed: %s", c.Name, c.Detail)
		}
	}
	info, err := svc.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.Status != model.ClusterDbReplStatusReady {
		t.Errorf("status = %q, want ready", info.Status)
	}
}

func TestDbReplPreflightMissingReplUser(t *testing.T) {
	restore := setupDbReplTestDB(t)
	defer restore()

	masterAgent := &fakeDbAgent{
		mysqlCnf:   "[mysqld]\nserver-id=1000\nlog-bin=mysql-bin\nbinlog_format=ROW\ngtid_mode=ON\n",
		binlogFile: "mysql-bin.000001", binlogPos: "1234",
		users: []map[string]interface{}{{"username": "root", "host": "localhost"}},
	}
	replicaAgent := &fakeDbAgent{mysqlCnf: "[mysqld]\nserver-id=2001\n"}
	masterSrv := httptest.NewServer(masterAgent.handler())
	defer masterSrv.Close()
	replicaSrv := httptest.NewServer(replicaAgent.handler())
	defer replicaSrv.Close()

	masterNode := insertTestNode(t, "master", masterSrv.URL, model.ClusterNodeStatusOnline)
	replicaNode := insertTestNode(t, "replica", replicaSrv.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterDbReplicationService()
	id := createTestDbRepl(t, svc, masterNode.ID, replicaNode.ID)
	checks, err := svc.Preflight(id)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	found := false
	for _, c := range checks {
		if c.Name == "複製帳號存在" {
			found = true
			if c.Status != "manual" {
				t.Errorf("repl user check = %q, want manual", c.Status)
			}
		}
	}
	if !found {
		t.Errorf("no repl user check in preflight results")
	}
}

func TestDbReplConfigureMaster(t *testing.T) {
	restore := setupDbReplTestDB(t)
	defer restore()

	masterAgent := &fakeDbAgent{mysqlCnf: "[mysqld]\nport=3306\n"}
	replicaAgent := &fakeDbAgent{mysqlCnf: "[mysqld]\nport=3306\n"}
	masterSrv := httptest.NewServer(masterAgent.handler())
	defer masterSrv.Close()
	replicaSrv := httptest.NewServer(replicaAgent.handler())
	defer replicaSrv.Close()

	masterNode := insertTestNode(t, "master", masterSrv.URL, model.ClusterNodeStatusOnline)
	replicaNode := insertTestNode(t, "replica", replicaSrv.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterDbReplicationService()
	id := createTestDbRepl(t, svc, masterNode.ID, replicaNode.ID)
	if err := svc.ConfigureMaster(id); err != nil {
		t.Fatalf("ConfigureMaster: %v", err)
	}
	seen := map[string]string{}
	for _, v := range masterAgent.varsSeen {
		if p, ok := v["param"].(string); ok {
			seen[p] = ""
			if val, ok := v["value"]; ok {
				seen[p] = jsonString(val)
			}
		}
	}
	for _, want := range []string{"server-id", "log-bin", "binlog_format", "gtid_mode", "enforce_gtid_consistency"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("master variables update missing %s (seen %v)", want, seen)
		}
	}
	if seen["binlog_format"] != "ROW" || seen["gtid_mode"] != "ON" {
		t.Errorf("master variables wrong: %v", seen)
	}
	if len(replicaAgent.varsSeen) == 0 {
		t.Errorf("replica server-id was not written")
	}
	info, _ := svc.Get(id)
	if info.MasterServerID == 0 || info.ReplicaServerID == 0 || info.MasterServerID == info.ReplicaServerID {
		t.Errorf("server ids not recorded distinctly: master %d replica %d", info.MasterServerID, info.ReplicaServerID)
	}
}

func jsonString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestDbReplConfigureMasterRefusesDirtyBinlog(t *testing.T) {
	restore := setupDbReplTestDB(t)
	defer restore()

	// Binlog already on, GTID off: anonymous transactions may sit in the
	// binlog, so forcing gtid_mode=ON could fail master startup.
	masterAgent := &fakeDbAgent{mysqlCnf: "[mysqld]\nserver-id=1000\nlog-bin=mysql-bin\ngtid_mode=OFF\n"}
	replicaAgent := &fakeDbAgent{mysqlCnf: "[mysqld]\n"}
	masterSrv := httptest.NewServer(masterAgent.handler())
	defer masterSrv.Close()
	replicaSrv := httptest.NewServer(replicaAgent.handler())
	defer replicaSrv.Close()

	masterNode := insertTestNode(t, "master", masterSrv.URL, model.ClusterNodeStatusOnline)
	replicaNode := insertTestNode(t, "replica", replicaSrv.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterDbReplicationService()
	id := createTestDbRepl(t, svc, masterNode.ID, replicaNode.ID)
	if err := svc.ConfigureMaster(id); err == nil {
		t.Fatalf("ConfigureMaster should refuse dirty-binlog GTID enablement")
	}
	info, _ := svc.Get(id)
	if info.Status != model.ClusterDbReplStatusError {
		t.Errorf("status = %q, want error", info.Status)
	}
}

func TestDbReplPromoteSwapsTopology(t *testing.T) {
	restore := setupDbReplTestDB(t)
	defer restore()

	masterAgent := &fakeDbAgent{mysqlCnf: "[mysqld]\nserver-id=1000\n"}
	replicaAgent := &fakeDbAgent{mysqlCnf: "[mysqld]\nserver-id=2001\n"}
	masterSrv := httptest.NewServer(masterAgent.handler())
	defer masterSrv.Close()
	replicaSrv := httptest.NewServer(replicaAgent.handler())
	defer replicaSrv.Close()

	masterNode := insertTestNode(t, "master", masterSrv.URL, model.ClusterNodeStatusOnline)
	replicaNode := insertTestNode(t, "replica", replicaSrv.URL, model.ClusterNodeStatusOnline)

	svc := NewIClusterDbReplicationService()
	id := createTestDbRepl(t, svc, masterNode.ID, replicaNode.ID)
	// Move the topology to running as if replication had been confirmed.
	if err := global.DB.Model(&model.ClusterDbReplication{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": model.ClusterDbReplStatusRunning}).Error; err != nil {
		t.Fatalf("set running: %v", err)
	}
	fo, err := svc.Promote(dto.ClusterDbPromoteReq{ID: id, Reason: "master down", ManualStepKeys: []string{"stop-replica"}})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if fo.FromNodeID != masterNode.ID || fo.ToNodeID != replicaNode.ID {
		t.Errorf("failover %+v", fo)
	}
	info, _ := svc.Get(id)
	if info.Status != model.ClusterDbReplStatusFailedOver {
		t.Errorf("status = %q, want failed_over", info.Status)
	}
	if info.MasterNodeID != replicaNode.ID || info.ReplicaNodeID != masterNode.ID {
		t.Errorf("topology not swapped: master %d replica %d", info.MasterNodeID, info.ReplicaNodeID)
	}
	// Replica read_only must have been flipped OFF (restart) via variables.
	flipped := false
	for _, v := range replicaAgent.varsSeen {
		if p, ok := v["param"].(string); ok && p == "read_only" {
			flipped = true
		}
	}
	if !flipped {
		t.Errorf("replica read_only was not flipped off")
	}
	failovers, err := svc.ListFailovers(id)
	if err != nil || len(failovers) != 1 {
		t.Errorf("ListFailovers = %d, err %v", len(failovers), err)
	}
}
