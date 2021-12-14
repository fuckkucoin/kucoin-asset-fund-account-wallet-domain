package trexecgate

import "fmt"

var (
	// ErrLeaseNotFound 租约不存在或已被清除。
	ErrLeaseNotFound = fmt.Errorf("trexecgate: plan exec lease not found")
	// ErrLeaseExpired 租约已超过 TTL，不得放行 MarkSent/Acked。
	ErrLeaseExpired = fmt.Errorf("trexecgate: plan exec lease expired")
	// ErrLeaseBlocked 租约因漂移或人工封堵已进入 blocked 终态。
	ErrLeaseBlocked = fmt.Errorf("trexecgate: plan exec lease blocked")
	// ErrDriftExceeded 执行前复核时热账水位偏离超过容忍阈值。
	ErrDriftExceeded = fmt.Errorf("trexecgate: hot inventory drift exceeded tolerance")
	// ErrIllegalTransition 状态机非法跃迁；调用方须保证按 FSM 流转。
	ErrIllegalTransition = fmt.Errorf("trexecgate: illegal lease state transition")
	// ErrDuplicateExec 同 plan 幂等键已有活跃租约，不得双开 Exec。
	ErrDuplicateExec = fmt.Errorf("trexecgate: duplicate exec for same plan idem key")
	// ErrLeaseIDMismatch MarkSent/Acked 携带的 LeaseID 与 Exec 绑定不符。
	ErrLeaseIDMismatch = fmt.Errorf("trexecgate: lease id mismatch on terminal bind")
	// ErrSnapshotUnavailable 无法取得当前热库存快照，拒绝绑定或复核。
	ErrSnapshotUnavailable = fmt.Errorf("trexecgate: hot inventory snapshot unavailable")
	// ErrInvalidInput 参数校验失败。
	ErrInvalidInput = fmt.Errorf("trexecgate: invalid input")
)
