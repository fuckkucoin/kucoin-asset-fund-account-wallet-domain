package feepathgate

import "fmt"

var (
	// ErrUnavailable 表示引擎依赖的存储或子系统未就绪。
	ErrUnavailable = fmt.Errorf("feepathgate unavailable")
	// ErrPathConflict 同一 business_key 已被另一 Kind 持有路径租约；
	// 本次 Post 被拒，调用方不得静默重试，必须人工介入或走冲突确认流程。
	ErrPathConflict = fmt.Errorf("feepathgate: path conflict — same business_key bound to opposite Kind")
	// ErrStaleGen 世代 CAS 失败：调用方持有的 generation 号已过时，
	// 必须重新读取当前世代再决策。
	ErrStaleGen = fmt.Errorf("feepathgate: stale generation — CAS rejected")
	// ErrLeaseNotFound 指定 lease_id 或 business_key 不存在。
	ErrLeaseNotFound = fmt.Errorf("feepathgate: lease not found")
	// ErrDualHitExists 同一 business_key 已存在 DualHit 记录（open/proven 状态），
	// 不得重复发现，应走 Resolve 流程。
	ErrDualHitExists = fmt.Errorf("feepathgate: dual-hit already exists")
	// ErrHedgeImbalance 费收取与返佣贷记对冲不平衡：
	// gross_take ≠ gross_rebate 或 fee_take ≠ fee_rebate
	// 或集合哈希与已持久化的 HedgeProof 不符。
	ErrHedgeImbalance = fmt.Errorf("feepathgate: hedge imbalance — fee/rebate amounts do not balance")
	// ErrProofMissing 对冲证明记录缺失；在证明未固化前不得将 lease 标记为 settled。
	ErrProofMissing = fmt.Errorf("feepathgate: hedge proof missing — cannot settle without proof")
	// ErrProofVerify 对冲证明验签失败：字段集合哈希与存档摘要不符。
	ErrProofVerify = fmt.Errorf("feepathgate: hedge proof verification failed")
	// ErrReplayConflict 幂等复用时 Kind/Asset/Gross 三元组不全等；
	// 冲突必须升级为 ErrPathConflict，禁止静默覆盖。
	ErrReplayConflict = fmt.Errorf("feepathgate: replay conflict — Kind/Asset/Gross mismatch")
	// ErrKind 传入的 Kind 字符串不合法。
	ErrKind = fmt.Errorf("feepathgate: kind must be 'take' or 'rebate'")
	// ErrBusinessKey business_key 形态非法或为空。
	ErrBusinessKey = fmt.Errorf("feepathgate: business_key invalid")
	// ErrAsset 资产标识非法或为空。
	ErrAsset = fmt.Errorf("feepathgate: asset invalid")
	// ErrAmount 金额字符串形态非法。
	ErrAmount = fmt.Errorf("feepathgate: amount invalid")
	// ErrLeaseBound 租约已进入 bound/settled/conflicted 终态，不可二次绑定。
	ErrLeaseBound = fmt.Errorf("feepathgate: lease already bound — cannot re-bind")
	// ErrWorkerConflict worker 标识与当前持有者不匹配，禁止跨 worker 释放租约。
	ErrWorkerConflict = fmt.Errorf("feepathgate: worker mismatch — cannot release another worker's lease")
)
