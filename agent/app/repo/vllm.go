package repo

import "github.com/1Panel-dev/1Panel/agent/app/model"

type VllmRepo struct{}

type IVllmRepo interface {
	PageModels(page, size int, opts ...DBOption) (int64, []model.VllmModel, error)
	ListModels(opts ...DBOption) ([]model.VllmModel, error)
	GetModel(opts ...DBOption) (*model.VllmModel, error)
	SaveModel(item *model.VllmModel) error
	DeleteModel(opts ...DBOption) error

	PageRecipes(page, size int, opts ...DBOption) (int64, []model.VllmRecipe, error)
	ListRecipes(opts ...DBOption) ([]model.VllmRecipe, error)
	GetRecipe(opts ...DBOption) (*model.VllmRecipe, error)
	SaveRecipe(item *model.VllmRecipe) error
	DeleteRecipe(opts ...DBOption) error

	PageDeployments(page, size int, opts ...DBOption) (int64, []model.VllmDeployment, error)
	ListDeployments(opts ...DBOption) ([]model.VllmDeployment, error)
	GetDeployment(opts ...DBOption) (*model.VllmDeployment, error)
	SaveDeployment(item *model.VllmDeployment) error
	DeleteDeployment(opts ...DBOption) error
}

func NewIVllmRepo() IVllmRepo { return &VllmRepo{} }

func pageOf[T any](page, size int, opts ...DBOption) (int64, []T, error) {
	var items []T
	var count int64
	db := getDb(opts...).Model(new(T))
	if err := db.Count(&count).Error; err != nil {
		return 0, nil, err
	}
	err := db.Order("created_at desc").Limit(size).Offset(size * (page - 1)).Find(&items).Error
	return count, items, err
}

func listOf[T any](opts ...DBOption) ([]T, error) {
	var items []T
	err := getDb(opts...).Order("created_at desc").Find(&items).Error
	return items, err
}

func firstOf[T any](opts ...DBOption) (*T, error) {
	var item T
	if err := getDb(opts...).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (VllmRepo) PageModels(page, size int, opts ...DBOption) (int64, []model.VllmModel, error) {
	return pageOf[model.VllmModel](page, size, opts...)
}
func (VllmRepo) ListModels(opts ...DBOption) ([]model.VllmModel, error) {
	return listOf[model.VllmModel](opts...)
}
func (VllmRepo) GetModel(opts ...DBOption) (*model.VllmModel, error) {
	return firstOf[model.VllmModel](opts...)
}
func (VllmRepo) SaveModel(item *model.VllmModel) error { return getDb().Save(item).Error }
func (VllmRepo) DeleteModel(opts ...DBOption) error {
	return getDb(opts...).Delete(&model.VllmModel{}).Error
}

func (VllmRepo) PageRecipes(page, size int, opts ...DBOption) (int64, []model.VllmRecipe, error) {
	return pageOf[model.VllmRecipe](page, size, opts...)
}
func (VllmRepo) ListRecipes(opts ...DBOption) ([]model.VllmRecipe, error) {
	return listOf[model.VllmRecipe](opts...)
}
func (VllmRepo) GetRecipe(opts ...DBOption) (*model.VllmRecipe, error) {
	return firstOf[model.VllmRecipe](opts...)
}
func (VllmRepo) SaveRecipe(item *model.VllmRecipe) error { return getDb().Save(item).Error }
func (VllmRepo) DeleteRecipe(opts ...DBOption) error {
	return getDb(opts...).Delete(&model.VllmRecipe{}).Error
}

func (VllmRepo) PageDeployments(page, size int, opts ...DBOption) (int64, []model.VllmDeployment, error) {
	return pageOf[model.VllmDeployment](page, size, opts...)
}
func (VllmRepo) ListDeployments(opts ...DBOption) ([]model.VllmDeployment, error) {
	return listOf[model.VllmDeployment](opts...)
}
func (VllmRepo) GetDeployment(opts ...DBOption) (*model.VllmDeployment, error) {
	return firstOf[model.VllmDeployment](opts...)
}
func (VllmRepo) SaveDeployment(item *model.VllmDeployment) error { return getDb().Save(item).Error }
func (VllmRepo) DeleteDeployment(opts ...DBOption) error {
	return getDb(opts...).Delete(&model.VllmDeployment{}).Error
}
