package repo

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
)

type ClusterSyncRepo struct{}

type IClusterSyncRepo interface {
	CreateTask(task *model.ClusterSyncTask) error
	UpdateTask(id uint, vars map[string]interface{}) error
	GetTask(id uint) (model.ClusterSyncTask, error)
	PageTasks(page, size int, opts ...global.DBOption) (int64, []model.ClusterSyncTask, error)
	CreateRecord(record *model.ClusterSyncRecord) error
	ListRecords(taskID uint) ([]model.ClusterSyncRecord, error)
	LastSyncStatusByTarget(targetNodeID uint) (model.ClusterSyncRecord, error)
}

func NewIClusterSyncRepo() IClusterSyncRepo {
	return &ClusterSyncRepo{}
}

func (u *ClusterSyncRepo) CreateTask(task *model.ClusterSyncTask) error {
	return global.DB.Create(task).Error
}

func (u *ClusterSyncRepo) UpdateTask(id uint, vars map[string]interface{}) error {
	return global.DB.Model(&model.ClusterSyncTask{}).Where("id = ?", id).Updates(vars).Error
}

func (u *ClusterSyncRepo) GetTask(id uint) (model.ClusterSyncTask, error) {
	var task model.ClusterSyncTask
	err := global.DB.First(&task, id).Error
	return task, err
}

func (u *ClusterSyncRepo) PageTasks(page, size int, opts ...global.DBOption) (int64, []model.ClusterSyncTask, error) {
	var tasks []model.ClusterSyncTask
	db := global.DB.Model(&model.ClusterSyncTask{})
	for _, opt := range opts {
		db = opt(db)
	}
	count := int64(0)
	db = db.Count(&count)
	err := db.Order("created_at desc").Limit(size).Offset(size * (page - 1)).Find(&tasks).Error
	return count, tasks, err
}

func (u *ClusterSyncRepo) CreateRecord(record *model.ClusterSyncRecord) error {
	return global.DB.Create(record).Error
}

func (u *ClusterSyncRepo) ListRecords(taskID uint) ([]model.ClusterSyncRecord, error) {
	var records []model.ClusterSyncRecord
	err := global.DB.Where("task_id = ?", taskID).Order("created_at asc").Find(&records).Error
	return records, err
}

func (u *ClusterSyncRepo) LastSyncStatusByTarget(targetNodeID uint) (model.ClusterSyncRecord, error) {
	var record model.ClusterSyncRecord
	err := global.DB.Where("target_node_id = ?", targetNodeID).Order("created_at desc").First(&record).Error
	return record, err
}
