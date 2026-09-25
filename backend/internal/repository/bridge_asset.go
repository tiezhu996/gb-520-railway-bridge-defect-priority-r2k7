package repository

import (
	"context"
	"time"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"gorm.io/gorm"
)

// BridgeAssetRepository owns all persistence operations for 桥梁资产.
type BridgeAssetRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.BridgeAsset], error)
	Get(context.Context, uint) (model.BridgeAsset, error)
	Create(context.Context, *model.BridgeAsset) error
	Update(context.Context, uint, uint, *model.BridgeAsset) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	ListByFacility(context.Context, string) ([]model.BridgeAsset, error)
	RestrictActiveByFacilityInTx(ctx context.Context, tx *gorm.DB, facility string) (int64, error)
}

type bridgeAssetRepository struct {
	db    *gorm.DB
	store *Store[model.BridgeAsset]
}

func NewBridgeAssetRepository(db *gorm.DB) BridgeAssetRepository {
	return &bridgeAssetRepository{db: db, store: NewStore[model.BridgeAsset](db)}
}

func (r *bridgeAssetRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.BridgeAsset], error) {
	return r.store.List(ctx, q)
}
func (r *bridgeAssetRepository) Get(ctx context.Context, id uint) (model.BridgeAsset, error) {
	return r.store.Get(ctx, id)
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

// ListByFacility returns every non-deleted bridge operating at the given
// facility. Priority finalization uses it to cascade service restrictions.
func (r *bridgeAssetRepository) ListByFacility(ctx context.Context, facility string) ([]model.BridgeAsset, error) {
	items := make([]model.BridgeAsset, 0)
	err := r.db.WithContext(ctx).
		Where("facility = ?", facility).
		Order("id ASC").Find(&items).Error
	return items, err
}

// RestrictActiveByFacilityInTx atomically moves every active bridge at the
// facility to restricted, bumping the optimistic-lock version. It returns the
// number of bridges updated. Closed/retired/restricted rows are untouched and
// must be pre-validated by the service.
func (r *bridgeAssetRepository) RestrictActiveByFacilityInTx(ctx context.Context, tx *gorm.DB, facility string) (int64, error) {
	result := tx.WithContext(ctx).Model(&model.BridgeAsset{}).
		Where("facility = ? AND status = ?", facility, model.BridgeStatusActive).
		Updates(map[string]any{"status": model.BridgeStatusRestricted, "version": gorm.Expr("version + 1"), "updated_at": time.Now().UTC()})
	return result.RowsAffected, result.Error
}
