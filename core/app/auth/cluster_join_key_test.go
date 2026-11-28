package auth

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/utils/common"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
)

func TestParseClusterJoinWhiteList(t *testing.T) {
	got, err := ParseClusterJoinWhiteList(" 10.0.0.5, 192.168.0.0/16 ,")
	if err != nil || got != "10.0.0.5\n192.168.0.0/16" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := ParseClusterJoinWhiteList(""); err == nil {
		t.Fatal("empty list must fail")
	}
	if _, err := ParseClusterJoinWhiteList("10.0.0.5;rm -rf"); err == nil {
		t.Fatal("garbage must fail")
	}
}

// The key written by `1panel cluster-join` must be usable exactly like one
// created from the API key page: decryptable with the panel key,
// signature-valid and limited to the master IP. Joining again retires the
// previous key.
func TestCreateClusterJoinKey(t *testing.T) {
	db, err := common.GetDBWithPath(filepath.Join(t.TempDir(), "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.APIKey{}); err != nil {
		t.Fatal(err)
	}
	const encKey = "0123456789abcdef"
	first, err := CreateClusterJoinKey(db, "cluster-master", "10.0.0.5", encKey)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateClusterJoinKey(db, "cluster-master", "10.0.0.5", encKey)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(second) != 32 {
		t.Fatalf("unexpected secrets %q %q", first, second)
	}
	var active []model.APIKey
	db.Where("deleted_at IS NULL").Find(&active)
	if len(active) != 1 {
		t.Fatalf("want 1 active key, got %d", len(active))
	}
	key := active[0]
	if key.Status != "Enable" || key.IPWhiteList != "10.0.0.5" || key.OwnerType != APIKeyOwnerPanelAdmin {
		t.Fatalf("unexpected key %+v", key)
	}
	if err := ValidateAPIKeyState(key, time.Now()); err != nil {
		t.Fatal(err)
	}
	plain, err := encrypt.DecryptAPIKey(key.SecretCiphertext, key.ID, encKey)
	if err != nil || plain != second {
		t.Fatalf("decrypt: %q %v", plain, err)
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	if !IsValid1PanelToken(GenerateHMACSHA256(second, "1panel:"+ts), ts, plain) {
		t.Fatal("signature must validate")
	}
	if !IsIPInWhiteList("10.0.0.5", key.IPWhiteList) || IsIPInWhiteList("10.0.0.6", key.IPWhiteList) {
		t.Fatal("whitelist must only accept the master")
	}
	var retired int64
	db.Model(&model.APIKey{}).Where("deleted_at IS NOT NULL AND status = ? AND secret_ciphertext = ''", "Revoked").Count(&retired)
	if retired != 1 {
		t.Fatalf("previous key must be retired, got %d", retired)
	}
}
