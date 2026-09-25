package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/config"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type ruleHarness struct {
	db         *gorm.DB
	bridges    BridgeAssetService
	defects    DefectFindingService
	priorities PriorityDecisionService
	bridgeRepo repository.BridgeAssetRepository
}

func newRuleHarness(t *testing.T) *ruleHarness {
	t.Helper()
	dsn := fmt.Sprintf("file:rules-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.BridgeAsset{}, &model.DefectFinding{},
		&model.PriorityDecision{}, &model.PriorityDecisionRevision{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	bridgeRepo := repository.NewBridgeAssetRepository(db)
	defectRepo := repository.NewDefectFindingRepository(db)
	priorityRepo := repository.NewPriorityDecisionRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	return &ruleHarness{
		db:         db,
		bridges:    NewBridgeAssetService(bridgeRepo, defectRepo, security),
		defects:    NewDefectFindingService(defectRepo, security),
		priorities: NewPriorityDecisionService(db, priorityRepo, bridgeRepo, security),
		bridgeRepo: bridgeRepo,
	}
}

const testFacility = "K42 桥梁"

func createTestBridge(t *testing.T, h *ruleHarness, code, status string) model.BridgeAsset {
	t.Helper()
	ctx := context.Background()
	item, err := h.bridges.Create(ctx, dto.CreateBridgeAsset{
		Code: code, Name: "桥梁 " + code, Facility: testFacility, Owner: "运行一组",
		Category: "结构", RiskLevel: "high", MetricUnit: "score",
		EffectiveAt: time.Now().UTC(), Evidence: "现场检查记录",
	}, "operator", "req-bridge-"+code)
	if err != nil {
		t.Fatalf("create bridge %s: %v", code, err)
	}
	// Seed non-active bridges through the real state machine so optimistic
	// versions stay consistent.
	for _, target := range bridgeSeedPath(status) {
		item, err = h.bridges.Transition(ctx, item.ID, dto.TransitionRequest{
			Status: target, ExpectedVersion: item.Version, Reason: "测试预置桥梁状态 " + target,
		}, "operator", model.RoleOperator, "req-bridge-seed-"+code+"-"+target)
		if err != nil {
			t.Fatalf("seed bridge %s to %s: %v", code, target, err)
		}
	}
	return item
}

func bridgeSeedPath(status string) []string {
	switch status {
	case model.BridgeStatusActive:
		return nil
	case model.BridgeStatusRestricted:
		return []string{model.BridgeStatusRestricted}
	case model.BridgeStatusClosed:
		return []string{model.BridgeStatusClosed}
	default:
		panic("unsupported seed bridge status " + status)
	}
}

func createTestDefect(t *testing.T, h *ruleHarness, code, status string) model.DefectFinding {
	t.Helper()
	ctx := context.Background()
	item, err := h.defects.Create(ctx, dto.CreateDefectFinding{
		Code: code, Name: "缺陷 " + code, Facility: testFacility, Owner: "检查班组",
		Category: "梁体", RiskLevel: "high", MetricUnit: "score",
		EffectiveAt: time.Now().UTC(), Evidence: "裂缝照片与量测数据",
	}, "operator", "req-defect-"+code)
	if err != nil {
		t.Fatalf("create defect %s: %v", code, err)
	}
	for _, target := range defectSeedPath(status) {
		item, err = h.defects.Transition(ctx, item.ID, dto.TransitionRequest{
			Status: target, ExpectedVersion: item.Version, Reason: "测试预置缺陷状态 " + target,
		}, "operator", "req-defect-seed-"+code+"-"+target)
		if err != nil {
			t.Fatalf("seed defect %s to %s: %v", code, target, err)
		}
	}
	return item
}

func defectSeedPath(status string) []string {
	switch status {
	case "new":
		return nil
	case "verified":
		return []string{"verified"}
	case "monitoring":
		return []string{"verified", "monitoring"}
	case "mitigated":
		return []string{"verified", "monitoring", "mitigated"}
	default:
		panic("unsupported seed defect status " + status)
	}
}

func createTestDecision(t *testing.T, h *ruleHarness, code string) model.PriorityDecision {
	t.Helper()
	created, err := h.priorities.Create(context.Background(), dto.CreatePriorityDecision{
		Code: code, Name: "处置决定 " + code, Facility: testFacility, Owner: "调度室",
		Category: "限速", RiskLevel: "critical", MetricValue: 90, MetricUnit: "score",
		EffectiveAt: time.Now().UTC(), Evidence: "缺陷证据已核对", RelatedCode: "DF-001",
	}, "operator", "req-decision-"+code)
	if err != nil {
		t.Fatalf("create decision %s: %v", code, err)
	}
	return created
}

// 定稿为限速时，同设施正常运行的桥梁必须自动进入受限。
func TestFinalizeRestrictMovesActiveBridgeToRestricted(t *testing.T) {
	h := newRuleHarness(t)
	ctx := context.Background()
	bridge := createTestBridge(t, h, "BA-R1", model.BridgeStatusActive)
	decision := createTestDecision(t, h, "PD-R1")

	finalized, err := h.priorities.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "restrict", ExpectedVersion: decision.Version, Reason: "现场确认需要限速",
	}, "reviewer", model.RoleReviewer, "req-finalize-restrict")
	if err != nil {
		t.Fatalf("finalize restrict: %v", err)
	}
	if finalized.Status != "restrict" {
		t.Fatalf("decision status = %q, want restrict", finalized.Status)
	}
	updated, err := h.bridgeRepo.Get(ctx, bridge.ID)
	if err != nil {
		t.Fatalf("reload bridge: %v", err)
	}
	if updated.Status != model.BridgeStatusRestricted {
		t.Fatalf("bridge status = %q, want restricted after restrict finalization", updated.Status)
	}
	if updated.Version != bridge.Version+1 {
		t.Fatalf("bridge version = %d, want %d after cascade", updated.Version, bridge.Version+1)
	}
}

// 定稿为立即处置时，同设施正常运行的桥梁同样进入受限。
func TestFinalizeUrgentMovesActiveBridgeToRestricted(t *testing.T) {
	h := newRuleHarness(t)
	ctx := context.Background()
	bridge := createTestBridge(t, h, "BA-U1", model.BridgeStatusActive)
	decision := createTestDecision(t, h, "PD-U1")

	if _, err := h.priorities.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "urgent", ExpectedVersion: decision.Version, Reason: "立即处置",
	}, "admin", model.RoleAdmin, "req-finalize-urgent"); err != nil {
		t.Fatalf("finalize urgent: %v", err)
	}
	updated, err := h.bridgeRepo.Get(ctx, bridge.ID)
	if err != nil {
		t.Fatalf("reload bridge: %v", err)
	}
	if updated.Status != model.BridgeStatusRestricted {
		t.Fatalf("bridge status = %q, want restricted after urgent finalization", updated.Status)
	}
}

// 同设施桥梁已经关闭时，限速/立即处置卡片不能定稿放行。
func TestFinalizeRejectedWhenBridgeClosed(t *testing.T) {
	h := newRuleHarness(t)
	ctx := context.Background()
	bridge := createTestBridge(t, h, "BA-C1", model.BridgeStatusClosed)
	decision := createTestDecision(t, h, "PD-C1")

	_, err := h.priorities.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "restrict", ExpectedVersion: decision.Version, Reason: "尝试对关闭桥梁放行",
	}, "reviewer", model.RoleReviewer, "req-finalize-closed")
	if !errors.Is(err, ErrBridgeClosed) {
		t.Fatalf("want ErrBridgeClosed, got %v", err)
	}
	if !strings.Contains(err.Error(), bridge.Code) {
		t.Fatalf("error should identify the closed bridge, got %v", err)
	}
	// 决定仍是草稿，桥梁仍关闭。
	decisionAfter, _ := h.priorities.Get(ctx, decision.ID)
	if decisionAfter.Status != "draft" {
		t.Fatalf("decision status = %q, want draft after rejection", decisionAfter.Status)
	}
	bridgeAfter, _ := h.bridgeRepo.Get(ctx, bridge.ID)
	if bridgeAfter.Status != model.BridgeStatusClosed {
		t.Fatalf("bridge status = %q, want closed untouched", bridgeAfter.Status)
	}
}

// observe（观察）定稿不应改动任何桥梁状态。
func TestFinalizeObserveDoesNotRestrictBridge(t *testing.T) {
	h := newRuleHarness(t)
	ctx := context.Background()
	bridge := createTestBridge(t, h, "BA-O1", model.BridgeStatusActive)
	decision := createTestDecision(t, h, "PD-O1")

	if _, err := h.priorities.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "observe", ExpectedVersion: decision.Version, Reason: "保持观察",
	}, "reviewer", model.RoleReviewer, "req-finalize-observe"); err != nil {
		t.Fatalf("finalize observe: %v", err)
	}
	updated, err := h.bridgeRepo.Get(ctx, bridge.ID)
	if err != nil {
		t.Fatalf("reload bridge: %v", err)
	}
	if updated.Status != model.BridgeStatusActive {
		t.Fatalf("bridge status = %q, want active untouched after observe", updated.Status)
	}
}

// 受限桥梁恢复正常运行时，同设施仍有确认阶段缺陷则拒绝，并写明剩余条数。
func TestRestrictedBridgeCannotRestoreWhileDefectsUnconfirmed(t *testing.T) {
	h := newRuleHarness(t)
	ctx := context.Background()
	bridge := createTestBridge(t, h, "BA-X1", model.BridgeStatusRestricted)
	createTestDefect(t, h, "DF-N1", "new")
	verified := createTestDefect(t, h, "DF-V1", "verified")
	// 另一条已进入监测阶段的缺陷不应计入阻塞。
	createTestDefect(t, h, "DF-M1", "monitoring")

	_, err := h.bridges.Transition(ctx, bridge.ID, dto.TransitionRequest{
		Status: model.BridgeStatusActive, ExpectedVersion: bridge.Version, Reason: "尝试恢复正常运行",
	}, "reviewer", model.RoleReviewer, "req-restore-blocked")
	if !errors.Is(err, ErrDefectsUnconfirmed) {
		t.Fatalf("want ErrDefectsUnconfirmed, got %v", err)
	}
	if !strings.Contains(err.Error(), "还剩 2 条") {
		t.Fatalf("error must state remaining defect count (2), got %v", err)
	}

	// 确认阶段缺陷推进到监测后，阻塞数下降；但只要还剩 new 缺陷仍不能恢复。
	monitoring, err := h.defects.Transition(ctx, verified.ID, dto.TransitionRequest{
		Status: "monitoring", ExpectedVersion: verified.Version, Reason: "确认完成转入监测",
	}, "operator", "req-defect-monitor")
	if err != nil {
		t.Fatalf("move verified defect to monitoring: %v", err)
	}
	if monitoring.Status != "monitoring" {
		t.Fatalf("defect status = %q, want monitoring", monitoring.Status)
	}
	_, err = h.bridges.Transition(ctx, bridge.ID, dto.TransitionRequest{
		Status: model.BridgeStatusActive, ExpectedVersion: bridge.Version, Reason: "再次尝试恢复",
	}, "reviewer", model.RoleReviewer, "req-restore-still-blocked")
	if !errors.Is(err, ErrDefectsUnconfirmed) || !strings.Contains(err.Error(), "还剩 1 条") {
		t.Fatalf("want 1 remaining defect error, got %v", err)
	}
}

// 全部确认阶段缺陷都推进后，复核人可恢复；操作员无权恢复。
func TestRestrictedBridgeRestoreRules(t *testing.T) {
	h := newRuleHarness(t)
	ctx := context.Background()
	bridge := createTestBridge(t, h, "BA-X2", model.BridgeStatusRestricted)
	createTestDefect(t, h, "DF-M2", "monitoring")

	_, err := h.bridges.Transition(ctx, bridge.ID, dto.TransitionRequest{
		Status: model.BridgeStatusActive, ExpectedVersion: bridge.Version, Reason: "操作员尝试恢复",
	}, "operator", model.RoleOperator, "req-restore-operator")
	if !errors.Is(err, ErrBridgeRestoreRole) {
		t.Fatalf("operator restore want ErrBridgeRestoreRole, got %v", err)
	}

	restored, err := h.bridges.Transition(ctx, bridge.ID, dto.TransitionRequest{
		Status: model.BridgeStatusActive, ExpectedVersion: bridge.Version, Reason: "缺陷已进入监测，恢复运行",
	}, "reviewer", model.RoleReviewer, "req-restore-reviewer")
	if err != nil {
		t.Fatalf("reviewer restore: %v", err)
	}
	if restored.Status != model.BridgeStatusActive {
		t.Fatalf("bridge status = %q, want active", restored.Status)
	}
	if restored.UnconfirmedDefectCount != 0 {
		t.Fatalf("unconfirmed count = %d, want 0", restored.UnconfirmedDefectCount)
	}
}

// 桥梁工作台列表附带每座桥的未确认缺陷条数。
func TestBridgeListIncludesUnconfirmedDefectCount(t *testing.T) {
	h := newRuleHarness(t)
	createTestBridge(t, h, "BA-L1", model.BridgeStatusActive)
	createTestDefect(t, h, "DF-L1", "new")
	createTestDefect(t, h, "DF-L2", "new")
	createTestDefect(t, h, "DF-L3", "mitigated")

	page, err := h.bridges.List(context.Background(), dto.PageQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list bridges: %v", err)
	}
	var found bool
	for _, item := range page.Items {
		if item.Code == "BA-L1" {
			found = true
			if item.UnconfirmedDefectCount != 2 {
				t.Fatalf("unconfirmed count = %d, want 2 (mitigated excluded)", item.UnconfirmedDefectCount)
			}
		}
	}
	if !found {
		t.Fatal("BA-L1 missing from bridge list")
	}
}
