package viptiergate

import "fmt"

var (
	// ErrUnavailable 服务或存储层未初始化。
	ErrUnavailable = fmt.Errorf("viptiergate: service unavailable")
	// ErrUserInvalid 用户 ID 非法。
	ErrUserInvalid = fmt.Errorf("viptiergate: user id invalid")
	// ErrAssetInvalid 资产标识非法。
	ErrAssetInvalid = fmt.Errorf("viptiergate: asset invalid")
	// ErrTierInvalid 档位值非法。
	ErrTierInvalid = fmt.Errorf("viptiergate: tier invalid")
	// ErrStaleGen CAS 世代号已过期，另一并发操作抢先更新了世代。
	// 调用方应读取最新世代后重试；重试次数建议不超过 3 次。
	ErrStaleGen = fmt.Errorf("viptiergate: stale generation, CAS conflict")
	// ErrInflightFreeze 存在未结的在途出金冻额，降档/清钉被互斥阻塞。
	// 只有所有在途冻额释放后，降档操作才允许接着。
	ErrInflightFreeze = fmt.Errorf("viptiergate: inflight freeze mutex: downgrade blocked")
	// ErrInflightQuote 存在未到期或未结算的 QuoteBind，降档被阻塞。
	ErrInflightQuote = fmt.Errorf("viptiergate: inflight quote bind present: downgrade blocked")
	// ErrLeaseNotFound 指定 lease_id 不存在。
	ErrLeaseNotFound = fmt.Errorf("viptiergate: tier lease not found")
	// ErrLeaseStale 操作目标租约的状态已被其他操作推进，不符合当前期望状态。
	ErrLeaseStale = fmt.Errorf("viptiergate: tier lease stale or already terminal")
	// ErrQuoteBindNotFound 指定 quote_bind_id 不存在。
	ErrQuoteBindNotFound = fmt.Errorf("viptiergate: quote bind not found")
	// ErrQuoteGenMismatch 提交结算的 QuoteBind 所携带的世代令牌与用户当前世代不匹配。
	// 档位发生切换后旧世代 Quote 必须拒绝结算。
	ErrQuoteGenMismatch = fmt.Errorf("viptiergate: quote generation mismatch: tier switched after quote issued")
	// ErrQuoteExpired QuoteBind TTL 已过期，必须重新报价。
	ErrQuoteExpired = fmt.Errorf("viptiergate: quote bind expired")
	// ErrDriftCaseNotFound 指定 drift_case_id 不存在。
	ErrDriftCaseNotFound = fmt.Errorf("viptiergate: drift case not found")
	// ErrDriftBlocked 该用户存在未关闭的漂移案，后续抬档结算被挡写直至四眼收口。
	ErrDriftBlocked = fmt.Errorf("viptiergate: drift case open: settlement blocked pending four-eye sign-off")
	// ErrProofMissing 漂移案尚无有效证明，不得关闭。
	ErrProofMissing = fmt.Errorf("viptiergate: drift proof missing: cannot close case")
	// ErrProofDigestMismatch 证明字段与摘要重计算不一致，证明已被篡改。
	ErrProofDigestMismatch = fmt.Errorf("viptiergate: drift proof digest mismatch")
	// ErrProofNotFound 指定 proof_id 不存在。
	ErrProofNotFound = fmt.Errorf("viptiergate: drift proof not found")
	// ErrFourEyeRequired 操作需要两名不同的操作员确认（四眼原则），当前仅有单人确认。
	ErrFourEyeRequired = fmt.Errorf("viptiergate: four-eye sign-off required: two distinct operators needed")
	// ErrDuplicateConfirm 同一操作员重复提交确认。
	ErrDuplicateConfirm = fmt.Errorf("viptiergate: duplicate four-eye confirm from same operator")
	// ErrAmountInvalid 金额字符串非法（空值/科学计数/负数）。
	ErrAmountInvalid = fmt.Errorf("viptiergate: amount invalid")
	// ErrPendingQueueFull 待切换队列已达容量上限，暂时无法入队。
	ErrPendingQueueFull = fmt.Errorf("viptiergate: pending tier-switch queue full")
)
