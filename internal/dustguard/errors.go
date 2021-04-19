package dustguard

import "errors"

var (
	// ErrAddressQuarantined 目标地址处于隔离中，禁止分配。
	ErrAddressQuarantined = errors.New("address is quarantined; allocation blocked")
	// ErrObserveBlocked 观察地址已被防护挡贷记票锁定，不得静默入账。
	ErrObserveBlocked = errors.New("observe blocked by dust guard; guard ticket opened")
	// ErrForensicsWindow 仍处于取证窗口内，不允许释放隔离。
	ErrForensicsWindow = errors.New("address still in forensics window; release denied")
	// ErrAlreadyQuarantined 地址已有活跃隔离案，不得重复开案。
	ErrAlreadyQuarantined = errors.New("address already has an active quarantine case")
	// ErrRotateNotApproved FamMEMO 共享地址轮换未获双人复核批准。
	ErrRotateNotApproved = errors.New("shared-memo rotate requires dual approval; not yet approved")
	// ErrUserRotateRejected 用户侧轮换 FamMEMO 共享地址被拒；须运维双人路径。
	ErrUserRotateRejected = errors.New("user-initiated rotate of shared-memo address is rejected; ops dual-approve path required")
	// ErrNoFreeSlot 库存中找不到可用地址槽，无法完成轮换。
	ErrNoFreeSlot = errors.New("no free address slot available for rotate")
	// ErrCaseNotFound 找不到指定的隔离案。
	ErrCaseNotFound = errors.New("quarantine case not found")
	// ErrPlanNotFound 找不到指定的轮换计划。
	ErrPlanNotFound = errors.New("rotate plan not found")
	// ErrPolicyNotFound 找不到指定策略。
	ErrPolicyNotFound = errors.New("guard policy not found")
	// ErrTicketNotFound 找不到指定防护单。
	ErrTicketNotFound = errors.New("guard ticket not found")
	// ErrInvalidTransition 非法状态跳转。
	ErrInvalidTransition = errors.New("invalid quarantine state transition")
	// ErrSameActor 开案人不能自批；须换人复核（与 custody 语义一致）。
	ErrSameActor = errors.New("same actor cannot approve own request; must be different ops")
	// ErrPolicyDisabled 策略未启用，操作被拒。
	ErrPolicyDisabled = errors.New("guard policy is disabled")
	// ErrCaseClosed 隔离案已关闭，不接受进一步状态变更。
	ErrCaseClosed = errors.New("quarantine case is already closed")
	// ErrBadCaseState 当前案件状态与所要求操作不匹配。
	ErrBadCaseState = errors.New("quarantine case state does not permit this operation")
)
