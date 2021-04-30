package feeoracle

import (
	"errors"
	"fmt"
	"account-assets.kucoin.net/funding/internal/quote"
)

var (
	// ErrUnavailable oracle 服务不可用（未配置源或 nil 接收者）。
	ErrUnavailable = fmt.Errorf("fee oracle unavailable")
	// ErrStale 价格 tick 超龄，不允许用于报价。
	ErrStale = fmt.Errorf("fee oracle tick stale")
	// ErrMissing 指定链没有任何 tick 记录。
	ErrMissing = fmt.Errorf("fee oracle tick missing")
	// ErrDeviation 多源 tick 偏差超过阈值，报价被熔断。
	ErrDeviation = fmt.Errorf("fee oracle source deviation exceeded threshold")
	// ErrCircuit 熔断器已打开，拒绝放行报价请求。
	ErrCircuit = fmt.Errorf("fee oracle circuit open")
	// ErrNoSources 没有为该链配置任何活跃的 fee 源。
	ErrNoSources = fmt.Errorf("fee oracle no active sources for chain")
	// ErrModel 链费率模型输入参数不合法。
	ErrModel = fmt.Errorf("fee model input invalid")
	// ErrChainFamily 未知的链族类型（不在 UTXO/EVM/TRON/SOL/GENERIC 之中）。
	ErrChainFamily = fmt.Errorf("unknown chain fee family")
	// ErrVersionNotFound 找不到指定代次 ID 的报价快照。
	ErrVersionNotFound = fmt.Errorf("quote version not found")
	// ErrVersionExpired 报价代次已过期，必须重新请求报价。
	ErrVersionExpired = fmt.Errorf("quote version expired; re-quote required")
	// ErrVersionBound 报价代次已绑定到出金单，不可重复绑定。
	ErrVersionBound = fmt.Errorf("quote version already bound to a withdraw")
	// ErrVersionConflict 绑定请求的出金单与已绑定的出金单不一致。
	ErrVersionConflict = fmt.Errorf("quote version withdraw id conflict")
	// ErrVersionSettled 报价代次已核算完成，状态不可再变更。
	ErrVersionSettled = fmt.Errorf("quote version already settled")
	// ErrConserve 报价不守恒：fee+net != gross。
	ErrConserve = fmt.Errorf("fee quote violates conservation: fee+net != gross")
	// ErrSource oracle 源配置不合法。
	ErrSource = fmt.Errorf("oracle source config invalid")
	// ErrNativeConvert 原生费用转换为用户资产计价失败。
	ErrNativeConvert = fmt.Errorf("fee oracle native→user asset conversion failed")
	ErrQuoteEmpty   = quote.ErrQuoteEmpty
	ErrQuoteStale   = quote.ErrQuoteStale
	ErrQuoteIllegal = quote.ErrQuoteIllegal
	ErrQuoteDiverge = quote.ErrQuoteDiverge
	ErrQuoteTimeout = quote.ErrQuoteTimeout
	ErrAssetRequired = errors.New("feeoracle asset required")
	ErrAmountInvalid = errors.New("feeoracle amount invalid")
	ErrLedgerRequired = errors.New("feeoracle ledger required")
	ErrAlreadyApplied = errors.New("feeoracle already applied")
)
