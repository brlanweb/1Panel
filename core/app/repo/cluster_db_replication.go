package repo

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
)

type ClusterDbReplicationRepo struct{}

type IClusterDbReplicationRepo interface {
	Create(rep *model.ClusterDbReplication) error
	Update(id uint, vars map[string]interface{}) error
	Get(id uint) (model.ClusterDbReplication, error)
	Delete(id uint) error
	List() ([]model.ClusterDbReplication, error)
	ListByMaster(masterNodeID uint, masterInstance, database string) ([]model.ClusterDbReplication, error)
	CreateFailover(f *model.ClusterDbFailover) error
	ListFailovers(replicationID uint) ([]model.ClusterDbFailover, error)
}

func NewIClusterDbReplicationRepo() IClusterDbReplicationRepo {
	return &ClusterDbReplicationRepo{}
}

func (u *ClusterDbReplicationRepo) Create(rep *model.ClusterDbReplication) error {
	return global.DB.Create(rep).Error
}

func (u *ClusterDbReplicationRepo) Update(id uint, vars map[string]interface{}) error {
	return global.DB.Model(&model.ClusterDbReplication{}).Where("id = ?", id).Updates(vars).Error
}

func (u *ClusterDbReplicationRepo) Get(id uint) (model.ClusterDbReplication, error) {
	var rep model.ClusterDbReplication
	err := global.DB.First(&rep, id).Error
	return rep, err
}

func (u *ClusterDbReplicationRepo) Delete(id uint) error {
	if err := global.DB.Where("replication_id = ?", id).Delete(&model.ClusterDbFailover{}).Error; err != nil {
		return err
	}
	return global.DB.Delete(&model.ClusterDbReplication{}, id).Error
}

func (u *ClusterDbReplicationRepo) List() ([]model.ClusterDbReplication, error) {
	var reps []model.ClusterDbReplication
	err := global.DB.Order("created_at desc").Find(&reps).Error
	return reps, err
}

func (u *ClusterDbReplicationRepo) ListByMaster(masterNodeID uint, masterInstance, database string) ([]model.ClusterDbReplication, error) {
	var reps []model.ClusterDbReplication
	err := global.DB.Where("master_node_id = ? AND master_instance = ? AND `database` = ?", masterNodeID, masterInstance, database).Find(&reps).Error
	return reps, err
}

func (u *ClusterDbReplicationRepo) CreateFailover(f *model.ClusterDbFailover) error {
	return global.DB.Create(f).Error
}

func (u *ClusterDbReplicationRepo) ListFailovers(replicationID uint) ([]model.ClusterDbFailover, error) {
	var list []model.ClusterDbFailover
	err := global.DB.Where("replication_id = ?", replicationID).Order("created_at desc").Find(&list).Error
	return list, err
}
