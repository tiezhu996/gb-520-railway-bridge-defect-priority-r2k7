package repository

import (
	"context"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"gorm.io/gorm"
)

// DefectFindingRepository owns all persistence operations for 缺陷发现.
type DefectFindingRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.DefectFinding], error)
	Get(context.Context, uint) (model.DefectFinding, error)
	Create(context.Context, *model.DefectFinding) error
	Update(context.Context, uint, uint, *model.DefectFinding) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	CountByFacilityAndStates(context.Context, []string, []string) (map[string]int64, error)
}

type defectFindingRepository struct {
	db    *gorm.DB
	store *Store[model.DefectFinding]
}

func NewDefectFindingRepository(db *gorm.DB) DefectFindingRepository {
	return &defectFindingRepository{db: db, store: NewStore[model.DefectFinding](db)}
}

func (r *defectFindingRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.DefectFinding], error) {
	return r.store.List(ctx, q)
}
func (r *defectFindingRepository) Get(ctx context.Context, id uint) (model.DefectFinding, error) {
	return r.store.Get(ctx, id)
}
func (r *defectFindingRepository) Create(ctx context.Context, item *model.DefectFinding) error {
	return r.store.Create(ctx, item)
}
func (r *defectFindingRepository) Update(ctx context.Context, id, version uint, item *model.DefectFinding) error {
	return r.store.Update(ctx, id, version, item)
}
func (r *defectFindingRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *defectFindingRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

// CountByFacilityAndStates groups live defects by facility, counting only
// those whose status is in states and whose facility is in facilities. The
// bridge workbench uses it once per page to attach the outstanding (still
// unconfirmed) defect count to every bridge without N+1 queries.
func (r *defectFindingRepository) CountByFacilityAndStates(ctx context.Context, facilities, states []string) (map[string]int64, error) {
	counts := make(map[string]int64)
	if len(facilities) == 0 || len(states) == 0 {
		return counts, nil
	}
	rows, err := r.db.WithContext(ctx).Model(&model.DefectFinding{}).
		Select("facility, COUNT(*) AS total").
		Where("facility IN ?", facilities).
		Where("status IN ?", states).
		Group("facility").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var facility string
		var total int64
		if err := rows.Scan(&facility, &total); err != nil {
			return nil, err
		}
		counts[facility] = total
	}
	return counts, rows.Err()
}
