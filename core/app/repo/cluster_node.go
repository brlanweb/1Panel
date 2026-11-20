package repo

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"gorm.io/gorm"
)

type ClusterNodeRepo struct{}

type IClusterNodeRepo interface {
	Get(opts ...global.DBOption) (model.ClusterNode, error)
	GetList(opts ...global.DBOption) ([]model.ClusterNode, error)
	Page(page, size int, opts ...global.DBOption) (int64, []model.ClusterNode, error)
	Create(node *model.ClusterNode) error
	Update(id uint, vars map[string]interface{}) error
	Delete(opts ...global.DBOption) error

	WithByInfo(info string) global.DBOption
}

func NewIClusterNodeRepo() IClusterNodeRepo {
	return &ClusterNodeRepo{}
}

func (u *ClusterNodeRepo) Get(opts ...global.DBOption) (model.ClusterNode, error) {
	var node model.ClusterNode
	db := global.DB
	for _, opt := range opts {
		db = opt(db)
	}
	err := db.First(&node).Error
	return node, err
}

func (u *ClusterNodeRepo) GetList(opts ...global.DBOption) ([]model.ClusterNode, error) {
	var nodes []model.ClusterNode
	db := global.DB.Model(&model.ClusterNode{})
	for _, opt := range opts {
		db = opt(db)
	}
	err := db.Find(&nodes).Error
	return nodes, err
}

func (u *ClusterNodeRepo) Page(page, size int, opts ...global.DBOption) (int64, []model.ClusterNode, error) {
	var nodes []model.ClusterNode
	db := global.DB.Model(&model.ClusterNode{})
	for _, opt := range opts {
		db = opt(db)
	}
	count := int64(0)
	db = db.Count(&count)
	err := db.Limit(size).Offset(size * (page - 1)).Find(&nodes).Error
	return count, nodes, err
}

func (u *ClusterNodeRepo) Create(node *model.ClusterNode) error {
	return global.DB.Create(node).Error
}

func (u *ClusterNodeRepo) Update(id uint, vars map[string]interface{}) error {
	return global.DB.Model(&model.ClusterNode{}).Where("id = ?", id).Updates(vars).Error
}

func (u *ClusterNodeRepo) Delete(opts ...global.DBOption) error {
	db := global.DB
	for _, opt := range opts {
		db = opt(db)
	}
	return db.Delete(&model.ClusterNode{}).Error
}

func (c *ClusterNodeRepo) WithByInfo(info string) global.DBOption {
	return func(g *gorm.DB) *gorm.DB {
		if len(info) == 0 {
			return g
		}
		return g.Where("name like ? or entry_url like ?", "%"+info+"%", "%"+info+"%")
	}
}
