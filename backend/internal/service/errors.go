package service

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrDecisionLocked    = errors.New("final priority decisions are immutable")
	ErrReviewRole        = errors.New("reviewer or admin role is required to finalize a priority")
	ErrSeparationOfDuty  = errors.New("priority preparer cannot approve the same decision")
	ErrNotDecisionOwner  = errors.New("only the preparer may edit this draft decision")
	ErrBridgeLinkedClosed = errors.New("same-facility bridge is closed or retired; the decision cannot be finalized for release")
	ErrBridgeMissing      = errors.New("no bridge asset found for the same facility; register the bridge before finalizing")
	ErrBridgeResumeRole   = errors.New("reviewer or admin role is required to resume normal bridge operation")
)

// ErrUnconfirmedDefects carries how many same-facility defects are still in
// the confirmation phase so the rejection message tells the reviewer exactly
// how many remain (请求要写明还剩几条).
type ErrUnconfirmedDefects struct {
	Remaining int64
}

func (e *ErrUnconfirmedDefects) Error() string {
	return fmt.Sprintf("bridge cannot resume normal operation: %d defect(s) at the same facility are still in new/verified confirmation", e.Remaining)
}

func (e *ErrUnconfirmedDefects) Is(target error) bool {
	_, ok := target.(*ErrUnconfirmedDefects)
	return ok
}
