package utxograph

import "fmt"

// Coin 一枚未花费或本交易里的输出。
type Coin struct {
	TxID string `json:"txid"`
	Vout int `json:"vout"`
	Amount string `json:"amount"`
	Address string `json:"address,omitempty"`
	Script string `json:"script,omitempty"`
}

// Tx 一笔带输入输出的交易。Fee 为空时用输入减输出。
type Tx struct {
	Chain string `json:"chain"`
	TxID  string `json:"txid"`
	Asset string `json:"asset,omitempty"`
	Fee   string `json:"fee,omitempty"`
	Inputs []Coin `json:"inputs"`
	Outputs []Coin `json:"outputs"`
}

// Labeled 打过标的输出。Credit 为假的币不能进用户钱包。

type Labeled struct {
	Coin
	UserID string `json:"user_id,omitempty"`
	Label  string `json:"label"`
	Credit bool `json:"credit"`
	Reason string `json:"reason,omitempty"`
}

// Ticket 未绑定、找零、守恒失败或合并歧义的工单。工单本身不是入账。
type Ticket struct {
	Kind string `json:"kind"`
	TxID string `json:"txid"`
	Vout int `json:"vout"`
	Amount string `json:"amount,omitempty"`
	Address string `json:"address,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	Note    string `json:"note,omitempty"`
	State string `json:"state"`
}

// Report 整笔交易的图结论。Incomplete 表示输入不够，不能判断找零。

type Report struct {
	Coins   []Labeled `json:"coins"`
	Tickets []Ticket  `json:"tickets,omitempty"`
	Fee     string    `json:"fee,omitempty"`
	ConserveOK bool `json:"conserve_ok"`
	Incomplete bool   `json:"incomplete"`
	Merge      bool   `json:"merge"`
	Reason     string `json:"reason,omitempty"`
}

// Limits 从链策略抄来的。找零永不入账，这里没有开关。
type Limits struct {
	Dust      string
	FeeMin    string
	FeeMaxBps int
	Scripts   []string
	Hot []string
}

// Binder 按地址或 txid:vout 认用户。认不到就当未绑定。

type Binder func(address, coinKey string) (user string, ok bool)

   func coinKey(txid string, vout int) string {
	  return fmt.Sprintf("%s:%d", txid, vout)
   }
