package brokerfence

import "fmt"

var (
	// ErrUnavailable 服务未初始化或依赖缺失。
	ErrUnavailable = fmt.Errorf("brokerfence: service unavailable")
	// ErrNotFound 目标实体不存在。
	ErrNotFound = fmt.Errorf("brokerfence: not found")
	// ErrSubNotFenced 操作要求子账已处于围栏态但实际未围栏。
	ErrSubNotFenced = fmt.Errorf("brokerfence: sub-account is not fenced")
	// ErrGenerationMismatch 世代号不匹配，写路径必须感知世代。
	ErrGenerationMismatch = fmt.Errorf("brokerfence: fence generation mismatch")
	// ErrMutexDenied 围栏窗口内无令牌写路径拒绝（fail-closed）。
	ErrMutexDenied = fmt.Errorf("brokerfence: alloc mutex denied – no fence token held")
	// ErrLeaseNotActive 围栏 Lease 未处于 active 态，操作非法。
	ErrLeaseNotActive = fmt.Errorf("brokerfence: fence lease is not active")
	// ErrLeaseSealConflict 围栏已封存，不得重复围栏。
	ErrLeaseSealConflict = fmt.Errorf("brokerfence: fence lease already sealed")
	// ErrConserveDrift 主子合计出现漂移，挡写直至收口。
	ErrConserveDrift = fmt.Errorf("brokerfence: master-sub conserve proof drifted – write blocked")
	// ErrProofMissing 解围需要守恒证明，但尚未产出。
	ErrProofMissing = fmt.Errorf("brokerfence: conserve proof missing – unfence refused")
	// ErrBypassCaseOpen 存在尚未关闭的旁路案件，拒绝提升世代。
	ErrBypassCaseOpen = fmt.Errorf("brokerfence: open bypass case blocks generation bump")
	// ErrDualApprovalRequired 旁路案件确认需要双人审核。
	ErrDualApprovalRequired = fmt.Errorf("brokerfence: bypass case requires dual approval")
	// ErrSameReviewer 双人审核不得同一人。
	ErrSameReviewer = fmt.Errorf("brokerfence: dual approval reviewers must differ")
	// ErrStateTransition 状态机转换非法。
	ErrStateTransition = fmt.Errorf("brokerfence: illegal state transition")
	// ErrTokenExpired 互斥令牌已过期。
	ErrTokenExpired = fmt.Errorf("brokerfence: fence mutex token expired")
	// ErrInput 入参不合法。
	ErrInput = fmt.Errorf("brokerfence: invalid input")
)
