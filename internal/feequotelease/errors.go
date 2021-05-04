package feequotelease

import "fmt"

var (
	// ErrUnavailable 服务不可用（未初始化或存储层故障）。
	ErrUnavailable = fmt.Errorf("fee quote lease: service unavailable")
	// ErrLeaseNotFound 租约不存在。
	ErrLeaseNotFound = fmt.Errorf("fee quote lease: lease not found")
	// ErrLeaseExpired 租约已过期（TTL 超时）。
	ErrLeaseExpired = fmt.Errorf("fee quote lease: lease expired")
	// ErrLeaseRevoked 租约已被吊销（Schedule 变更触发）。
	ErrLeaseRevoked = fmt.Errorf("fee quote lease: lease revoked")
	// ErrLeaseConsumed 租约已被消费（防止双扣）。
	ErrLeaseConsumed = fmt.Errorf("fee quote lease: lease already applied")
	// ErrNoLease Apply 未携带租约，fail-closed 拒绝。
	ErrNoLease = fmt.Errorf("fee quote lease: apply requires a valid lease (fail-closed)")
	// ErrSchGenDrift sch_gen 已抬升，档位漂移。
	ErrSchGenDrift = fmt.Errorf("fee quote lease: schedule generation drifted")
	// ErrFingerprintMismatch tiers 指纹不匹配。
	ErrFingerprintMismatch = fmt.Errorf("fee quote lease: tiers fingerprint mismatch")
	// ErrInvalidReq 请求参数非法。
	ErrInvalidReq = fmt.Errorf("fee quote lease: invalid request")
	// ErrScheduleNotFound 找不到对应 Schedule。
	ErrScheduleNotFound = fmt.Errorf("fee quote lease: schedule not found")
	// ErrGenNotFound 找不到 ScheduleGeneration 记录。
	ErrGenNotFound = fmt.Errorf("fee quote lease: schedule generation not found")
)
