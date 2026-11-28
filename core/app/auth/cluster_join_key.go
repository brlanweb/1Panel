package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/init/session/psession"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ParseClusterJoinWhiteList turns "1.2.3.4,10.0.0.0/8" into the newline
// separated list the API key middleware expects.
func ParseClusterJoinWhiteList(raw string) (string, error) {
	var items []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if net.ParseIP(item) == nil {
			if _, _, err := net.ParseCIDR(item); err != nil {
				return "", fmt.Errorf("invalid IP / CIDR %q", item)
			}
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return "", errors.New("the master IP list is empty")
	}
	return strings.Join(items, "\n"), nil
}

// CreateClusterJoinKey creates an independent API key of the panel admin
// for a cluster master, directly in core.db (used by `1panel
// cluster-join`, which runs next to the panel). A previous key with the
// same name is retired the same way the API key page deletes one, so a
// repeated join rotates the key. It returns the plaintext secret.
func CreateClusterJoinKey(db *gorm.DB, name, whiteList, encryptKey string) (string, error) {
	if strings.TrimSpace(name) == "" || whiteList == "" || encryptKey == "" {
		return "", errors.New("invalid cluster join key parameters")
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buf)
	hint := secret[:4] + "••••" + secret[len(secret)-4:]
	ownerType, ownerID := APIKeyOwnerPanelAdmin, psession.SuperAdminSessionUserID
	err := db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&model.APIKey{}).
			Where("owner_type = ? AND owner_id = ? AND active_name = ? AND deleted_at IS NULL", ownerType, ownerID, name).
			Updates(map[string]interface{}{
				"status": "Revoked", "revoked_at": now, "deleted_at": now, "active_name": nil,
				"secret_ciphertext": "", "key_hint": "", "allow_app_binding": false,
				"revision": gorm.Expr("revision + 1"),
			}).Error; err != nil {
			return err
		}
		requestID := uuid.NewString()
		key := model.APIKey{
			ID: uuid.NewString(), OwnerType: ownerType, OwnerID: ownerID,
			Name: name, ActiveName: &name, Description: "created by the cluster master (Terminal → Hosts → Join cluster)",
			RequestID: requestID, RequestHash: requestID, SecretVersion: 1, KeyHint: hint,
			Status: constant.StatusEnable, IPWhiteList: whiteList, APIKeyValidityTime: 120, Revision: 1,
		}
		cipher, err := encrypt.EncryptAPIKey(secret, key.ID, encryptKey)
		if err != nil {
			return err
		}
		key.SecretCiphertext = cipher
		return tx.Create(&key).Error
	})
	if err != nil {
		return "", err
	}
	return secret, nil
}
