package dustconv

import "time"

type Policy struct {
	Asset      string `json:"asset"`
	Floor      string `json:"floor"`
	MinConvert string `json:"min_convert"`
	Enabled    bool   `json:"enabled"`
}

type Rate struct {
	Asset string `json:"asset"`
	USDT  string `json:"usdt"`
}

type Convert struct {
	ID             string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	USDT   string `json:"usdt"`
	Rate   string `json:"rate"`
	Actor string `json:"actor"`
	At time.Time `json:"at"`
}

type ConvertReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Actor  string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
	DustQuoteLeaseID string `json:"dust_quote_lease_id,omitempty"`
}

type Book struct {
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
}

  func (p Policy) Copy() Policy   { return p }
   func (r Rate) Copy() Rate       { return r }
   func (c Convert) Copy() Convert { return c }

   func (b Book) Copy() Book { return b }
