package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/1Panel-dev/1Panel/core/i18n"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

// Emergency access for a node whose web login was locked by the cluster
// master (core/app/auth/managed_login.go). The panel reads the lock on every
// request, so the change takes effect immediately, no restart needed.
//
//	1pctl managed-login status | unlock
//	1pctl reset managed-login            (alias that works with old 1pctl)

func init() {
	RootCmd.AddCommand(managedLoginCmd)
	managedLoginCmd.AddCommand(managedLoginStatusCmd)
	managedLoginCmd.AddCommand(managedLoginUnlockCmd)
	resetCmd.AddCommand(resetManagedLoginCmd)
}

var managedLoginCmd = &cobra.Command{
	Use:   "managed-login",
	Short: "Show or remove the cluster-managed web login lock",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var managedLoginStatusCmd = &cobra.Command{
	Use: "status",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, ok := managedLoginDB("sudo 1pctl managed-login status")
		if !ok || db == nil {
			return nil
		}
		lock := getSettingByKey(db, "ManagedLoginLock")
		until, _ := strconv.ParseInt(getSettingByKey(db, "ManagedLoginUnlockUntil"), 10, 64)
		master := getSettingByKey(db, "ManagedLoginMaster")
		switch {
		case lock != "Enable":
			fmt.Println("web login: open (not locked)")
		case until > time.Now().Unix():
			fmt.Printf("web login: temporarily open until %s, then locked again\n", time.Unix(until, 0).Format("2006-01-02 15:04:05"))
		default:
			fmt.Println("web login: LOCKED (managed by the cluster master)")
		}
		if master != "" {
			fmt.Println("master: " + master)
		}
		return nil
	},
}

var managedLoginUnlockCmd = &cobra.Command{
	Use: "unlock",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runManagedLoginUnlock("sudo 1pctl managed-login unlock")
	},
}

var resetManagedLoginCmd = &cobra.Command{
	Use: "managed-login",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runManagedLoginUnlock("sudo 1pctl reset managed-login")
	},
}

func managedLoginDB(sudoCmd string) (*gorm.DB, bool) {
	i18n.UseI18nForCmd(language)
	if !isRoot() {
		fmt.Println(i18n.GetMsgWithMapForCmd("SudoHelper", map[string]interface{}{"cmd": sudoCmd}))
		return nil, false
	}
	db, err := loadDBConn("core.db")
	if err != nil {
		fmt.Println(err)
		return nil, false
	}
	return db, true
}

func runManagedLoginUnlock(sudoCmd string) error {
	db, ok := managedLoginDB(sudoCmd)
	if !ok || db == nil {
		return nil
	}
	if err := setSettingByKey(db, "ManagedLoginLock", "Disable"); err != nil {
		return err
	}
	if err := setSettingByKey(db, "ManagedLoginUnlockUntil", "0"); err != nil {
		return err
	}
	fmt.Println("web login unlocked; the cluster master can lock it again from the node list")
	return nil
}
