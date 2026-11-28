package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/auth"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

// `1panel cluster-join` is run by a cluster master over SSH (Terminal →
// Hosts → Join cluster). It creates a dedicated API key for the master,
// restricted to the master's IP, and prints it with the panel address
// parts on one marker line the master parses:
//
//	CLUSTER_JOIN_RESULT {"apiKey":"…","port":"…","ssl":false,…}
//
// Re-running it replaces the previous key of the same name, so joining a
// node again rotates its key. The running panel reads API keys from the
// database on every request: no restart is needed.

const clusterJoinMarker = "CLUSTER_JOIN_RESULT "

var (
	clusterJoinMasterIP string
	clusterJoinKeyName  string
	clusterJoinWait     int
)

func init() {
	clusterJoinCmd.Flags().StringVar(&clusterJoinMasterIP, "master-ip", "", "IP / CIDR list (comma separated) allowed to use the key")
	clusterJoinCmd.Flags().StringVar(&clusterJoinKeyName, "name", "cluster-master", "name of the API key")
	clusterJoinCmd.Flags().IntVar(&clusterJoinWait, "wait", 120, "seconds to wait for a freshly installed panel to finish its first start")
	RootCmd.AddCommand(clusterJoinCmd)
}

type clusterJoinResult struct {
	APIKey   string `json:"apiKey"`
	KeyHint  string `json:"keyHint"`
	Port     string `json:"port"`
	SSL      bool   `json:"ssl"`
	Entrance string `json:"entrance"`
	Version  string `json:"version"`
}

var clusterJoinCmd = &cobra.Command{
	Use:   "cluster-join",
	Short: "Create the API key a cluster master uses to manage this panel",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isRoot() {
			return errors.New("cluster-join must run as root")
		}
		whiteList, err := auth.ParseClusterJoinWhiteList(clusterJoinMasterIP)
		if err != nil {
			return err
		}
		db, err := loadDBConn("core.db")
		if err != nil {
			return err
		}
		encryptKey, err := clusterJoinWaitReady(db, time.Duration(clusterJoinWait)*time.Second)
		if err != nil {
			return err
		}
		secret, err := auth.CreateClusterJoinKey(db, strings.TrimSpace(clusterJoinKeyName), whiteList, encryptKey)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(clusterJoinResult{
			APIKey:   secret,
			KeyHint:  secret[:4] + "••••" + secret[len(secret)-4:],
			Port:     getSettingByKey(db, "ServerPort"),
			SSL:      getSettingByKey(db, "SSL") == constant.StatusEnable,
			Entrance: getSettingByKey(db, "SecurityEntrance"),
			Version:  getSettingByKey(db, "SystemVersion"),
		})
		fmt.Println(clusterJoinMarker + string(raw))
		return nil
	},
}

// clusterJoinWaitReady waits until the panel's first start has migrated
// the database (API key table + encryption key exist): right after an
// install the master calls this while the panel is still booting.
func clusterJoinWaitReady(db *gorm.DB, wait time.Duration) (string, error) {
	deadline := time.Now().Add(wait)
	for {
		key := getSettingByKey(db, "EncryptKey")
		if key != "" && db.Migrator().HasTable(&model.APIKey{}) && db.Migrator().HasColumn(&model.APIKey{}, "DeletedAt") {
			return key, nil
		}
		if time.Now().After(deadline) {
			return "", errors.New("the panel database is not ready (API keys unsupported or the panel has not started); upgrade or start the panel and retry")
		}
		time.Sleep(2 * time.Second)
	}
}
