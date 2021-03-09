package brokerbook

import "time"

type Priv string

const (
	PrivTrade Priv = "trade"
	PrivTransfer Priv = "transfer"
	PrivWithdraw Priv = "withdraw"
)

  func ParsePriv(s string) (Priv, error) {
	   switch Priv(s) {
	   case PrivTrade,
		  PrivTransfer, PrivWithdraw:
		   return Priv(s), nil
	   default:
		   return "", ErrPriv
	  }
  }

type Master struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Label string `json:"label"`
	OpenedAt  time.Time `json:"opened_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Sub struct {
	ID       string `json:"id"`
	MasterID string `json:"master_id"`
	UserID string `json:"user_id"`
	Label  string `json:"label"`
	Privileges []Priv `json:"privileges"`
	Fenced     bool   `json:"fenced"`
	OpenedAt  time.Time `json:"opened_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Book struct {
	OwnerID string `json:"owner_id"`
	Asset   string `json:"asset"`
	Available string `json:"available"`
}

type Alloc struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	MasterID string `json:"master_id"`
	SubID string `json:"sub_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Dir   string `json:"dir"`
	Actor string `json:"actor"`
	At time.Time `json:"at"`
}

type Fill struct {
	ID    string `json:"id"`
	SubID string `json:"sub_id"`
	Asset    string    `json:"asset"`
	Realized string    `json:"realized"`
	Note     string    `json:"note,omitempty"`
	At       time.Time `json:"at"`
}

type Rollup struct {
	MasterID string `json:"master_id"`
	Asset string `json:"asset"`
	Total string `json:"total"`
	BySub map[string]string `json:"by_sub,omitempty"`
	At    time.Time         `json:"at"`
}

type OpenMasterReq struct {
	UserID string `json:"user_id"`
	Label  string `json:"label"`
}

type OpenSubReq struct {
	MasterID string `json:"master_id"`
	UserID string `json:"user_id"`
	Label      string `json:"label"`
	Privileges []Priv `json:"privileges"`
}

type AllocReq struct {
	MasterID string `json:"master_id"`
	SubID  string `json:"sub_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Dir    string `json:"dir"`
	Actor  string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
	// FenceGeneration 与 FenceToken 为围栏世代校验扩展字段。
	// 非围栏场景可不填，围栏期内必填且必须与当前世代匹配。
	FenceGeneration int64 `json:"fence_generation,omitempty"`
	FenceToken string `json:"fence_token,omitempty"`
}

   func (m Master) Copy() Master { return m }

   func (s Sub) Copy() Sub {
	   cp := s
	   if s.Privileges != nil {
		  cp.Privileges = append([]Priv(nil), s.Privileges...)
	  }
	   return cp
  }

   func (b Book) Copy() Book { return b }

   func (a Alloc) Copy() Alloc { return a }

  func (f Fill) Copy() Fill { return f }

   func (r Rollup) Copy() Rollup {
	  cp := r
	  if r.BySub != nil {
		  cp.BySub = make(map[string]string, len(r.BySub))
		  for k, v := range r.BySub {
			   cp.BySub[k] = v
		  }
	   }
	   return cp
  }
