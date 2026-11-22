package auth

import (
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
)

// Managed login lock.
//
// A panel registered as a cluster node can have its own web login switched
// off by the master panel. The master keeps managing the node through the
// API key (HMAC headers), which never goes through the session login, so
// locking the login page does not affect cluster management.
//
//   - ManagedLoginLock:        Enable / Disable
//   - ManagedLoginUnlockUntil: unix seconds; while now < until the login is
//     temporarily open again (0 = no temporary window)
//   - ManagedLoginMaster:      display name of the managing master
//
// The state is read on every check, so `1pctl managed-login unlock` (which
// writes the database directly) takes effect without a restart.
const (
	SettingManagedLoginLock        = "ManagedLoginLock"
	SettingManagedLoginUnlockUntil = "ManagedLoginUnlockUntil"
	SettingManagedLoginMaster      = "ManagedLoginMaster"
)

type ManagedLoginState struct {
	Supported   bool   `json:"supported"`
	Locked      bool   `json:"locked"`
	Enabled     bool   `json:"enabled"`
	UnlockUntil int64  `json:"unlockUntil"`
	Master      string `json:"master"`
}

func LoadManagedLoginState() ManagedLoginState {
	settingRepo := repo.NewISettingRepo()
	lock, _ := settingRepo.GetValueByKey(SettingManagedLoginLock)
	untilStr, _ := settingRepo.GetValueByKey(SettingManagedLoginUnlockUntil)
	master, _ := settingRepo.GetValueByKey(SettingManagedLoginMaster)
	until, _ := strconv.ParseInt(strings.TrimSpace(untilStr), 10, 64)
	state := ManagedLoginState{
		Supported:   true,
		Enabled:     lock == constant.StatusEnable,
		UnlockUntil: until,
		Master:      master,
	}
	state.Locked = managedLoginLocked(state.Enabled, until, time.Now())
	if !state.Enabled || until <= time.Now().Unix() {
		state.UnlockUntil = 0
	}
	return state
}

func managedLoginLocked(enabled bool, unlockUntil int64, now time.Time) bool {
	if !enabled {
		return false
	}
	return unlockUntil <= 0 || now.Unix() >= unlockUntil
}

// IsManagedLoginLocked reports whether web login is currently refused.
func IsManagedLoginLocked() bool {
	if global.DB == nil {
		return false
	}
	return LoadManagedLoginState().Locked
}

// UpdateManagedLogin applies a lock change requested by the master panel.
//
//	locked=true                       lock now (drops all web sessions)
//	locked=false, unlockMinutes>0     open temporarily, lock again afterwards
//	locked=false, unlockMinutes<=0    remove the lock completely
func UpdateManagedLogin(locked bool, unlockMinutes int, master string) (ManagedLoginState, error) {
	settingRepo := repo.NewISettingRepo()
	if strings.TrimSpace(master) != "" {
		if err := settingRepo.UpdateOrCreate(SettingManagedLoginMaster, strings.TrimSpace(master)); err != nil {
			return ManagedLoginState{}, err
		}
	}
	lockValue := constant.StatusDisable
	until := int64(0)
	switch {
	case locked:
		lockValue = constant.StatusEnable
	case unlockMinutes > 0:
		lockValue = constant.StatusEnable
		until = time.Now().Add(time.Duration(unlockMinutes) * time.Minute).Unix()
	}
	if err := settingRepo.UpdateOrCreate(SettingManagedLoginLock, lockValue); err != nil {
		return ManagedLoginState{}, err
	}
	if err := settingRepo.UpdateOrCreate(SettingManagedLoginUnlockUntil, strconv.FormatInt(until, 10)); err != nil {
		return ManagedLoginState{}, err
	}
	if locked && global.SESSION != nil {
		_ = global.SESSION.Clean()
	}
	return LoadManagedLoginState(), nil
}
