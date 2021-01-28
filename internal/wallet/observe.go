// 链上观测结果，充值入账按这里的金额和资产。
// 作者: Likeo
// 日期: 2021.01.17
package wallet

import (
	"context"
	"github.com/shopspring/decimal"
)

const TxReorged TxStatus = "reorged"

// Observation 链上交易的金额、资产和收款方。入账按观测值结算。
type Observation struct {
	Hash string
	Asset string
	Amount    decimal.Decimal
	ToAddress string // 链上实际收款地址；充值必须等于该用户托管地址
	FromAddress string // 链上付款地址；观测层能给出时写入，供 KYT/旅行规则使用
	Memo    string // FamMEMO 备注（XLM/ATOM/EOS/TON）；漏填不入账
	Tag     string // 目的地标签（XRP destination_tag 等）；与 Memo 同值时双写
	TagName string // memo / destination_tag / comment
	Status  TxStatus
	Confirmations int
	Height    int64
	BlockHash string
	Reorged   bool
	OutputIndex int // UTXO vout 或账户模型 logIndex；入账自然键含此项
}

// Fork 控制链头：出块、回滚、覆盖观察金额。
type Fork interface {
	Mine(hashes ...string) int64
	Reorg(drop int) []string
	SetObserved(hash, asset string, amount decimal.Decimal)
	SetAutoMine(bool)
	Head() (height int64,
		blockHash string)
}

// TxBatchInspector 一次 RPC 拉多笔哈希。
type TxBatchInspector interface {
	InspectTxBatch(ctx context.Context, hashes []string) (map[string]Observation, error)
}
