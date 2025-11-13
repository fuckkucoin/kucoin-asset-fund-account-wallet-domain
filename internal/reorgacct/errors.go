package reorgacct

import "errors"

var (
	// ErrDupEvent 同一 (chain, tx_hash, depth) 三元组已存在；调用方应检查幂等键。
	ErrDupEvent = errors.New("reorgacct: duplicate reorg event")
	// ErrUnknownChain chain 字符串无法规范化。
	ErrUnknownChain = errors.New("reorgacct: unknown chain")
	// ErrBadDepth depth 非正数或回退量为负。
	ErrBadDepth = errors.New("reorgacct: bad depth value")
	// ErrNoOutput 指定的 (chain, tx_hash, output_index) 在影响盘点时找不到贷记记录。
	ErrNoOutput = errors.New("reorgacct: output not found in credit records")
	// ErrImpactSealed 该 ReorgEvent 的影响面已封存，禁止追加行。
	ErrImpactSealed = errors.New("reorgacct: impact rows sealed for this event")
	// ErrBadAmount 金额字符串无法解析为正十进制数。
	ErrBadAmount = errors.New("reorgacct: bad amount string")
	// ErrStoreUnavailable 持久化后端不可用（nil 指针或连接错误）。
	ErrStoreUnavailable = errors.New("reorgacct: store unavailable")
	// ErrEventNotFound 按 ID 查询事件时不存在。
	ErrEventNotFound = errors.New("reorgacct: event not found")
	// ErrScanBusy 同一 event_id 的盘点正在运行中，禁止并发重入。
	ErrScanBusy = errors.New("reorgacct: scan already running for this event")
	// ErrLiveHashCoexist 受影响的出金跟踪含 live-hash，禁止自动解冻，仅开事故单。
	ErrLiveHashCoexist = errors.New("reorgacct: live-hash coexists; manual review required")
)
