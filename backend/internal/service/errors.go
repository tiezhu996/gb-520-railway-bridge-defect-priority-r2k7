package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrDecisionLocked    = errors.New("final priority decisions are immutable")
	ErrReviewRole        = errors.New("reviewer or admin role is required to finalize a priority")
	ErrSeparationOfDuty  = errors.New("priority preparer cannot approve the same decision")
	ErrNotDecisionOwner  = errors.New("only the preparer may edit this draft decision")
	ErrBridgeClosed      = errors.New("同设施桥梁已关闭或停用，限速决定不能定稿放行")
	ErrBridgeRestoreRole = errors.New("只有复核人或管理员可以将受限桥梁恢复为正常运行")
	ErrDefectsUnconfirmed = errors.New("同设施仍有缺陷处于确认阶段，受限桥梁暂不能恢复正常运行")
)
