package quote

import "errors"

var (
	// ErrQuoteEmpty 报价缺失：空结果、零有效档、未初始化或非法数值。
	ErrQuoteEmpty = errors.New("quote empty")
	// ErrQuoteStale 报价超出 TTL 新鲜度窗口。
	ErrQuoteStale = errors.New("quote stale")
	// ErrQuoteIllegal 独立校验失败（与 Empty 并列，表驱动可锁定为本错误）。
	ErrQuoteIllegal = errors.New("quote illegal")
	// ErrPairRequired 币对或资产键缺失。
	ErrPairRequired = errors.New("quote pair required")
	// ErrClockRequired Confirm 路径必须注入时钟，禁止依赖隐式墙钟。
	ErrClockRequired = errors.New("quote clock required")
	// ErrSourceMasked 空或过期报价被标为 market，视为伪装。
	ErrSourceMasked = errors.New("quote source masked as market")
	// ErrHouseOnConfirm Confirm 调用栈不得取 house 调试价。
	ErrHouseOnConfirm = errors.New("house debug quote forbidden on confirm")
	// ErrQuoteDiverge 多源报价价差超阈或指纹不一致。
	ErrQuoteDiverge = errors.New("quote diverge")
	// ErrQuoteTimeout 多源采集超时或缺源，禁止静默取一家。
	ErrQuoteTimeout = errors.New("quote timeout")
)
