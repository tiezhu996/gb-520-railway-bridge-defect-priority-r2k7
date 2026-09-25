package repository

import (
	"context"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"gorm.io/gorm"
)

// BridgeAssetRepository owns all persistence operations for 桥梁资产.
type BridgeAssetRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.BridgeAsset], error)
	Get(context.Context, uint) (model.BridgeAsset, error)
	FindByFacility(context.Context, string) (model.BridgeAsset, error)
	Create(context.Context, *model.BridgeAsset) error
	Update(context.Context, uint, uint, *model.BridgeAsset) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
}

type bridgeAssetRepository struct {
	db    *gorm.DB
	store *Store[model.BridgeAsset]
}

func NewBridgeAssetRepository(gormDB *gorm.DB) BridgeAssetRepository {
	return &bridgeAssetRepository{db: gormDB, store: NewStore[model.BridgeAsset](gormDB)}
}

func (r *bridgeAssetRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.BridgeAsset], error) {
	return r.store.List(ctx, q)
}
func (r *bridgeAssetRepository) Get(ctx context.Context, id uint) (model.BridgeAsset, error) {
	return r.store.Get(ctx, id)
}
func (r *bridgeAssetRepository) FindByFacility(ctx context.Context, facility string) (model.BridgeAsset, error) {
	var item model.BridgeAsset
	err := r.db.WithContext(ctx).
		Where("facility = ?", facility).
		Order("id ASC").First(&item).Error
	return item, err
}
func (r *bridgeAssetRepository) Create(ctx context.Context, item *model.BridgeAsset) error {
	return r.store.Create(ctx, item)
}
func (r *bridgeAssetRepository) Update(ctx context.Context, id, version uint, item *model.BridgeAsset) error {
	return r.store.Update(ctx, id, version, item)
}
func (r *bridgeAssetRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *bridgeAssetRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}
