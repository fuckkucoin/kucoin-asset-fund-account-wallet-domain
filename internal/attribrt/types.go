package attribrt

import "time"

// Sight 观察线程或台席送来的一笔链上事实。金额不在这里改写。
type Sight struct {
	Chain string
	TxHash      string
	OutputIndex int
	Asset       string
	Amount      string
	From string
	To   string
	Memo    string
	Tag     string
	Script  string
	Network string
	UserHint      string
	PolicyVersion string
	Confirmations int
	Replay bool
	Graph  *GraphIn
	At     time.Time
	// Preview 只计算结论。不写碰撞索引、速率窗、图缓存和信封。
	Preview bool `json:"preview,omitempty"`
}

// GraphIn 可选的 UTXO 图。账户模型链留空。
type GraphIn struct {
	Fee string
	Inputs  []CoinIn
	Outputs []CoinIn
}

// CoinIn 图上的一枚币。

type CoinIn struct {
	TxID string
	Vout int
	Amount  string
	Address string
	Script  string
}

// Decision 结构化结论。EnvelopeID 供审计仓消费，本包不落审计仓。

type Decision struct {
	ID    string `json:"id"`
	Chain string `json:"chain"`
	TxHash string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
	Asset       string `json:"asset"`
	Amount      string `json:"amount"`
	UserID string `json:"user_id,omitempty"`
	Kind string `json:"kind,omitempty"`
	Confidence int `json:"confidence"`
	Band        string `json:"band"`
	Disposition string `json:"disposition"`
	Reasons []string `json:"reasons,omitempty"`
	PolicyVersion string `json:"policy_version"`
	Family        string `json:"family"`
	ChainadFamily string `json:"chainad_family,omitempty"`
	CollisionID   string `json:"collision_id,omitempty"`
	Label string `json:"label,omitempty"`
	EnvelopeID   string `json:"envelope_id,omitempty"`
	CreditIntent bool   `json:"credit_intent"`
	Replay       bool   `json:"replay"`
	At time.Time `json:"at"`
}

// Binder 只读绑定。实现方不得在 Lookup 里入账。

type Binder interface {
	Lookup(chain,
		kind, key string) (user string, ok bool)
}

// BandOf 把置信度收成三档。低于门槛的档位不能入账。
   func BandOf(conf, floor int) string {
	  if conf < floor {
		   return "refuse"
	   }
	   if conf < floor+500 {
		  return "review"
	  }
	   return "credit"
   }
