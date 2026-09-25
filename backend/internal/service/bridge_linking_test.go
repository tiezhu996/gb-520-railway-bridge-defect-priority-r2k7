package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/config"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type linkageStore struct {
	db       *gorm.DB
	bridges  repository.BridgeAssetRepository
	defects  repository.DefectFindingRepository
	priority PriorityDecisionService
	bridge   BridgeAssetService
	defect   DefectFindingService
}

func newLinkageStore(t *testing.T) linkageStore {
	t.Helper()
	dsn := fmt.Sprintf("file:linkage-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.BridgeAsset{}, &model.DefectFinding{}, &model.PriorityDecision{}, &model.PriorityDecisionRevision{}, &model.AuditLog{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	bridges := repository.NewBridgeAssetRepository(db)
	defects := repository.NewDefectFindingRepository(db)
	return linkageStore{
		db:       db,
		bridges:  bridges,
		defects:  defects,
		priority: NewPriorityDecisionService(repository.NewPriorityDecisionRepository(db), bridges, defects, security),
		bridge:   NewBridgeAssetService(bridges, defects, security),
		defect:   NewDefectFindingService(defects, security),
	}
}

func seedBridge(t *testing.T, store linkageStore, code, status string) model.BridgeAsset {
	t.Helper()
	bridge := model.BridgeAsset{
		BaseModel: model.BaseModel{Code: code, Name: "K42 桥梁", Status: status, Version: 1},
		Facility: "K42 bridge", Owner: "infrastructure team",
	}
	if err := store.bridges.Create(context.Background(), &bridge); err != nil {
		t.Fatalf("seed bridge: %v", err)
	}
	return bridge
}

func seedDefect(t *testing.T, store linkageStore, code, status string) model.DefectFinding {
	t.Helper()
	defect := model.DefectFinding{
		BaseModel: model.BaseModel{Code: code, Name: "跨层联动缺陷", Status: status, Version: 1},
		Facility: "K42 bridge", Owner: "infrastructure team",
	}
	if err := store.defects.Create(context.Background(), &defect); err != nil {
		t.Fatalf("seed defect: %v", err)
	}
	return defect
}

func finalizeDecision(t *testing.T, ctx context.Context, store linkageStore, level string) (model.PriorityDecision, uint) {
	t.Helper()
	created, err := store.priority.Create(ctx, priorityCreateInput("PD-LINK-"+level, "evidence"), "operator", "req-create")
	if err != nil {
		t.Fatalf("create decision: %v", err)
	}
	transition := dto.TransitionRequest{Status: level, ExpectedVersion: created.Version, Reason: "independent safety review"}
	finalized, err := store.priority.Transition(ctx, created.ID, transition, "reviewer", model.RoleReviewer, "req-final")
	if err != nil {
		t.Fatalf("finalize decision %s: %v", level, err)
	}
	return finalized, created.ID
}

// 定稿为 restrict/urgent 后，同设施 active 桥梁必须同事务进入 restricted。
func TestFinalizeRestrictMovesActiveBridgeToRestricted(t *testing.T) {
	store := newLinkageStore(t)
	ctx := context.Background()
	bridge := seedBridge(t, store, "BA-LINK-1", model.BridgeStatusActive)

	finalizeDecision(t, ctx, store, "restrict")

	updated, err := store.bridges.Get(ctx, bridge.ID)
	if err != nil {
		t.Fatalf("reload bridge: %v", err)
	}
	if updated.Status != model.BridgeStatusRestricted || updated.Version != 2 {
		t.Fatalf("bridge must follow restrict finalization, got status=%s version=%d", updated.Status, updated.Version)
	}
}

// observe 只做监测，不应改变桥梁状态。
func TestFinalizeObserveLeavesBridgeActive(t *testing.T) {
	store := newLinkageStore(t)
	ctx := context.Background()
	bridge := seedBridge(t, store, "BA-LINK-2", model.BridgeStatusActive)

	finalizeDecision(t, ctx, store, "observe")

	updated, err := store.bridges.Get(ctx, bridge.ID)
	if err != nil {
		t.Fatalf("reload bridge: %v", err)
	}
	if updated.Status != model.BridgeStatusActive || updated.Version != 1 {
		t.Fatalf("observe must not change the bridge, got status=%s version=%d", updated.Status, updated.Version)
	}
}

// 桥梁已经关闭或者停用，restrict/urgent 定稿不放行。
func TestFinalizeRejectedWhenBridgeClosedOrRetired(t *testing.T) {
	for _, status := range []string{model.BridgeStatusClosed, model.BridgeStatusRetired} {
		store := newLinkageStore(t)
		ctx := context.Background()
		seedBridge(t, store, "BA-LINK-CLOSED", status)
		created, err := store.priority.Create(ctx, priorityCreateInput("PD-LINK-C", "evidence"), "operator", "req-create")
		if err != nil {
			t.Fatalf("create decision: %v", err)
		}
		transition := dto.TransitionRequest{Status: "urgent", ExpectedVersion: created.Version, Reason: "independent safety review"}
		if _, err := store.priority.Transition(ctx, created.ID, transition, "reviewer", model.RoleReviewer, "req-final"); !errors.Is(err, ErrBridgeLinkedClosed) {
			t.Fatalf("finalize against %s bridge must fail with linked-closed error, got %v", status, err)
		}
		// 决定仍停留在 draft，没有产生终态。
		reloaded, err := store.priority.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("reload decision: %v", err)
		}
		if reloaded.Status != "draft" || len(reloaded.Revisions) != 1 {
			t.Fatalf("rejected finalization must leave decision in draft, got status=%s revisions=%d", reloaded.Status, len(reloaded.Revisions))
		}
	}
}

// 同设施没有桥梁时，restrict/urgent 不能定稿，避免限速决定找不到受控资产。
func TestFinalizeRejectedWhenBridgeMissing(t *testing.T) {
	store := newLinkageStore(t)
	ctx := context.Background()
	created, err := store.priority.Create(ctx, priorityCreateInput("PD-LINK-MISS", "evidence"), "operator", "req-create")
	if err != nil {
		t.Fatalf("create decision: %v", err)
	}
	transition := dto.TransitionRequest{Status: "restrict", ExpectedVersion: created.Version, Reason: "independent safety review"}
	if _, err := store.priority.Transition(ctx, created.ID, transition, "reviewer", model.RoleReviewer, "req-final"); !errors.Is(err, ErrBridgeMissing) {
		t.Fatalf("finalize without same-facility bridge must fail, got %v", err)
	}
}

// 已受限桥梁上的二次定稿（restrict/urgent）仍然允许，桥梁保持 restricted。
func TestFinalizeWhileBridgeAlreadyRestricted(t *testing.T) {
	store := newLinkageStore(t)
	ctx := context.Background()
	bridge := seedBridge(t, store, "BA-LINK-3", model.BridgeStatusRestricted)

	finalizeDecision(t, ctx, store, "urgent")

	updated, err := store.bridges.Get(ctx, bridge.ID)
	if err != nil {
		t.Fatalf("reload bridge: %v", err)
	}
	if updated.Status != model.BridgeStatusRestricted || updated.Version != 1 {
		t.Fatalf("already-restricted bridge must stay put, got status=%s version=%d", updated.Status, updated.Version)
	}
}

// 受限桥梁恢复运行：还有确认阶段缺陷时拒绝，错误必须写明还剩几条；
// 复核人角色要求；缺陷推进到 monitoring/mitigated 后复核人可恢复。
func TestRestrictedBridgeRecoveryGuardedByUnconfirmedDefects(t *testing.T) {
	store := newLinkageStore(t)
	ctx := context.Background()
	bridge := seedBridge(t, store, "BA-LINK-4", model.BridgeStatusRestricted)
	seedDefect(t, store, "DF-LINK-1", "new")
	seedDefect(t, store, "DF-LINK-2", "verified")

	recovery := dto.TransitionRequest{Status: model.BridgeStatusActive, ExpectedVersion: bridge.Version, Reason: "复核人确认缺陷已进入监测后恢复运行"}

	// operator 不能恢复：需要复核人角色。
	if _, err := store.bridge.Transition(ctx, bridge.ID, recovery, "operator", model.RoleOperator, "req-op"); !errors.Is(err, ErrBridgeResumeRole) {
		t.Fatalf("operator recovery must be rejected with role error, got %v", err)
	}

	// 复核人尝试恢复：仍有 2 条确认阶段缺陷，错误写明剩余条数。
	_, err := store.bridge.Transition(ctx, bridge.ID, recovery, "reviewer", model.RoleReviewer, "req-blocked")
	var unconfirmed *ErrUnconfirmedDefects
	if !errors.As(err, &unconfirmed) || unconfirmed.Remaining != 2 {
		t.Fatalf("recovery blocked while 2 defects unconfirmed, got %v", err)
	}

	// 工作台列表必须带出限速状态与未确认缺陷条数。
	page, err := store.bridge.List(ctx, dto.PageQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list bridges: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].UnconfirmedDefectCount != 2 || page.Items[0].Status != model.BridgeStatusRestricted {
		t.Fatalf("workbench must surface restricted status and remaining count, got %+v", page.Items)
	}

	// 缺陷推进到 monitoring / mitigated 后，确认阶段缺陷清零，复核人可恢复。
	// new -> monitoring；verified -> monitoring -> mitigated。
	newDefect := moveDefectByStatus(t, ctx, store, "new", "monitoring")
	verifiedDefect := moveDefectByStatus(t, ctx, store, "verified", "mitigated")
	_ = newDefect
	_ = verifiedDefect

	page, _ = store.bridge.List(ctx, dto.PageQuery{Page: 1, PageSize: 20})
	if page.Items[0].UnconfirmedDefectCount != 0 {
		t.Fatalf("expected no unconfirmed defects after progression, got %d", page.Items[0].UnconfirmedDefectCount)
	}
	resumed, err := store.bridge.Transition(ctx, bridge.ID, recovery, "reviewer", model.RoleReviewer, "req-resume")
	if err != nil {
		t.Fatalf("reviewer recovery after defects progress: %v", err)
	}
	if resumed.Status != model.BridgeStatusActive {
		t.Fatalf("bridge should be active again, got %s", resumed.Status)
	}
}

// moveDefectByStatus finds the single defect at from and drives it to target
// along the legal transition graph, returning the final state.
func moveDefectByStatus(t *testing.T, ctx context.Context, store linkageStore, from, target string) model.DefectFinding {
	t.Helper()
	defectsPage, err := store.defects.List(ctx, dto.PageQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list defects: %v", err)
	}
	var defect model.DefectFinding
	for _, candidate := range defectsPage.Items {
		if candidate.Status == from {
			defect = candidate
			break
		}
	}
	if defect.ID == 0 {
		t.Fatalf("no defect in status %s", from)
	}
	path := []string{target}
	if from == "verified" && target == "mitigated" {
		path = []string{"monitoring", "mitigated"}
	}
	for _, next := range path {
		current, getErr := store.defects.Get(ctx, defect.ID)
		if getErr != nil {
			t.Fatalf("reload defect: %v", getErr)
		}
		request := dto.TransitionRequest{Status: next, ExpectedVersion: current.Version, Reason: "缺陷处置推进离开确认阶段"}
		updated, transitionErr := store.defect.Transition(ctx, defect.ID, request, "operator", "req-def")
		if transitionErr != nil {
			t.Fatalf("move defect %s -> %s: %v", current.Status, next, transitionErr)
		}
		defect = updated
	}
	return defect
}
