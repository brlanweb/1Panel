package repo

import (
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"gorm.io/gorm"
)

// ComputeRepo is a small generic repository shared by the compute
// scheduling tables (they all only need list / get / create / update / delete).
type ComputeRepo[T any] struct{}

func (ComputeRepo[T]) Get(opts ...global.DBOption) (T, error) {
	var item T
	db := global.DB.Model(new(T))
	for _, opt := range opts {
		db = opt(db)
	}
	err := db.First(&item).Error
	return item, err
}

func (ComputeRepo[T]) List(opts ...global.DBOption) ([]T, error) {
	var items []T
	db := global.DB.Model(new(T))
	for _, opt := range opts {
		db = opt(db)
	}
	err := db.Find(&items).Error
	return items, err
}

func (ComputeRepo[T]) Page(page, size int, opts ...global.DBOption) (int64, []T, error) {
	var items []T
	db := global.DB.Model(new(T))
	for _, opt := range opts {
		db = opt(db)
	}
	count := int64(0)
	db = db.Count(&count)
	err := db.Limit(size).Offset(size * (page - 1)).Find(&items).Error
	return count, items, err
}

func (ComputeRepo[T]) Create(item *T) error {
	return global.DB.Create(item).Error
}

func (ComputeRepo[T]) Update(id uint, vars map[string]interface{}) error {
	return global.DB.Model(new(T)).Where("id = ?", id).Updates(vars).Error
}

func (ComputeRepo[T]) Delete(opts ...global.DBOption) error {
	db := global.DB
	for _, opt := range opts {
		db = opt(db)
	}
	return db.Delete(new(T)).Error
}

func (ComputeRepo[T]) Count(opts ...global.DBOption) (int64, error) {
	db := global.DB.Model(new(T))
	for _, opt := range opts {
		db = opt(db)
	}
	count := int64(0)
	err := db.Count(&count).Error
	return count, err
}

var (
	ComputeHostRepo     = ComputeRepo[model.ComputeHost]{}
	ComputeLinkRepo     = ComputeRepo[model.ComputeFabricLink]{}
	ComputeRuntimeRepo  = ComputeRepo[model.ComputeRuntime]{}
	ComputeProfileRepo  = ComputeRepo[model.ComputeProfile]{}
	ComputeInstanceRepo = ComputeRepo[model.ComputeInstance]{}
	ComputeJobRepo      = ComputeRepo[model.ComputeJob]{}
	ComputeConsumerRepo = ComputeRepo[model.ComputeConsumer]{}
	ComputePolicyRepo   = ComputeRepo[model.ComputePolicy]{}
)

func WithComputeField(field string, value interface{}) global.DBOption {
	return func(g *gorm.DB) *gorm.DB {
		return g.Where(field+" = ?", value)
	}
}

func WithComputeLike(fields []string, info string) global.DBOption {
	return func(g *gorm.DB) *gorm.DB {
		if len(info) == 0 || len(fields) == 0 {
			return g
		}
		cond := ""
		args := make([]interface{}, 0, len(fields))
		for i, f := range fields {
			if i > 0 {
				cond += " or "
			}
			cond += f + " like ?"
			args = append(args, "%"+info+"%")
		}
		return g.Where(cond, args...)
	}
}

func WithComputeOrder(order string) global.DBOption {
	return func(g *gorm.DB) *gorm.DB { return g.Order(order) }
}
