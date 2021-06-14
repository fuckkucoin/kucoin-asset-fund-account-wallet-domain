// @FileName: errors.go
// @Summary: 资金 mesh 接线错误集合。
// 重点：取消双入口、旁路拨号、配置漂移与关键写共用稳定哨兵。
// 风险点：不得把未知旁路或远端失败吞成成功。
// @Author: Hux
// @Created: 2021.06.13

package fundmesh

import "fmt"

var (
	ErrUnavailable  = fmt.Errorf("fundmesh unavailable")
	ErrInvalidInput = fmt.Errorf("fundmesh input invalid")
	ErrWithdrawIDRequired = fmt.Errorf("fundmesh withdraw id required")
	ErrWithdrawNotFound = fmt.Errorf("fundmesh withdraw not found")
	ErrActorDenied      = fmt.Errorf("fundmesh actor denied")
	ErrCancelDenied     = fmt.Errorf("fundmesh cancel denied")
	ErrCancelTerminal   = fmt.Errorf("fundmesh cancel terminal")
	ErrAlreadyBroadcast = fmt.Errorf("fundmesh withdraw already broadcast")
	ErrAlreadyConfirmed = fmt.Errorf("fundmesh withdraw already confirmed")
	ErrAlreadyFailed    = fmt.Errorf("fundmesh withdraw already failed")
	ErrIdemConflict     = fmt.Errorf("fundmesh cancel idempotency conflict")
	ErrSideCallUnknown = fmt.Errorf("fundmesh sidecall name unknown")
	ErrSideCallDenied = fmt.Errorf("fundmesh sidecall denied")
	ErrSideCallFailed = fmt.Errorf("fundmesh sidecall failed")
	ErrConfigDrift    = fmt.Errorf("fundmesh config drift")
	ErrConfigMissingKey     = fmt.Errorf("fundmesh config key missing")
	ErrEtcdUnreachable      = fmt.Errorf("fundmesh etcd unreachable")
	ErrCriticalWriteClosed  = fmt.Errorf("fundmesh critical write fail-closed")
	ErrLedgerRemoteRequired = fmt.Errorf("fundmesh remote ledger required")
	ErrLedgerRemoteFailed = fmt.Errorf("fundmesh remote ledger failed")
	ErrSilentLocalForbidden  = fmt.Errorf("fundmesh silent local ledger fallback forbidden")
	ErrObserveWriteForbidden = fmt.Errorf("fundmesh observe ping must not write")
	ErrFaultDomainClosed     = fmt.Errorf("fundmesh fault domain closed")
	ErrPolicySealed         = fmt.Errorf("fundmesh production fallback switch sealed off")
	ErrRepoConflict         = fmt.Errorf("fundmesh repo conflict")
	ErrRepoUnavailable      = fmt.Errorf("fundmesh repo unavailable")
	ErrOutboundIdemInvalid  = fmt.Errorf("fundmesh outbound idempotency key invalid")
	ErrOutboundIdemConflict = fmt.Errorf("fundmesh outbound idempotency conflict")
	ErrOutboundIdemRequired = fmt.Errorf("fundmesh outbound idempotency key required")
	ErrOutboundPayload = fmt.Errorf("fundmesh outbound payload invalid")
	ErrOutboundDenied  = fmt.Errorf("fundmesh outbound denied")
	ErrOutboundTerminal = fmt.Errorf("fundmesh outbound already terminal")
	ErrOutboundReplayClosed = fmt.Errorf("fundmesh outbound replay after cancel")
	ErrDoubleDebitForbidden = fmt.Errorf("fundmesh outbound double debit forbidden")
	ErrOutboundUnavailable  = fmt.Errorf("fundmesh outbound store unavailable")
)
