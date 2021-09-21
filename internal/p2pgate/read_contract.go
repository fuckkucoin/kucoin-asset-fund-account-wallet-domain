package p2pgate

// HoldREST 公开 REST 占用行。
type HoldREST struct {
	ID       string `json:"id"`
	UserID   string `json:"userId"`
	Currency string `json:"currency"`
	AccountType string `json:"accountType"`
	Amount      string `json:"amount"`
	Reason      string `json:"reason"`
	Kind        string `json:"kind"`
	BizRef      string `json:"bizRef,omitempty"`
	Open           bool   `json:"open"`
	Status         string `json:"status"`
	ParkedOnBridge bool   `json:"parkedOnBridge"`
	CreatedAt       int64  `json:"createdAt"`
	DebitTransferId string `json:"debitTransferId,omitempty"`
	ReleaseTransferId string `json:"releaseTransferId,omitempty"`
	LedgerMatched     bool   `json:"ledgerMatched"`
}

// FlowREST 公开 REST 流水行。

type FlowREST struct {
	ID string `json:"id"`
	UserID    string `json:"userId"`
	Currency  string `json:"currency"`
	Amount    string `json:"amount"`
	Direction string `json:"direction"`
	BizType   string `json:"bizType"`
	OrderId   string `json:"orderId,omitempty"`
	HoldId string `json:"holdId,omitempty"`
	ClientOid string `json:"clientOid"`
	Status    string `json:"status"`
	Side      string `json:"side"`
	Adjacent  bool   `json:"adjacent"`
	CreatedAt int64  `json:"createdAt"`
}

// MerchantREST 公开 REST 商家占用行。

type MerchantREST struct {
	HoldREST
	Remaining string `json:"remaining"`
}

// SummaryREST 公开 REST 摘要。
type SummaryREST struct {
	UserID    string `json:"userId"`
	OpenHolds int    `json:"openHolds"`
	FrozenTotal string            `json:"frozenTotal"`
	ByCurrency  map[string]string `json:"byCurrency"`
	SellOpen     int    `json:"sellOpen"`
	SellTotal    string `json:"sellTotal"`
	MerchantOpen int    `json:"merchantOpen"`
	MerchantTotal string `json:"merchantTotal"`
	ReleasedCount int `json:"releasedCount"`
	FlowCount int `json:"flowCount"`
	IntegrityOk bool `json:"integrityOk"`
}

  func HoldToREST(v HoldView) HoldREST {
	  return HoldREST{
		  ID: v.ID, UserID: v.UserID, Currency: v.Asset, AccountType: restBook(v.Book),
		  Amount: v.Amount, Reason: v.Reason, Kind: v.Kind, BizRef: v.Ref,
		   Open: v.Open, Status: v.Status, ParkedOnBridge: v.Parked,
		   CreatedAt: v.CreatedAtMs, DebitTransferId: v.DebitID, ReleaseTransferId: v.ReleaseID,
		  LedgerMatched: v.Matched,
	   }
  }

  func HoldsToREST(items []HoldView) []HoldREST {
	   out := make([]HoldREST, 0, len(items))
	  for _, v := range items {
		  out = append(out, HoldToREST(v))
	  }
	   return out
   }

   func FlowToREST(e FlowEntry) FlowREST {
	  return FlowREST{
		  ID:     e.TransferID,
		  UserID: e.UserID, Currency: e.Asset, Amount: e.Amount,
		   Direction: e.Direction, BizType: e.Action, OrderId: e.OrderID, HoldId: e.HoldID,
		  ClientOid: e.IdempotencyKey, Status: e.Status, Side: e.Sign,
		  Adjacent: e.Adjacent, CreatedAt: e.CreatedAtMs,
	  }
  }

   func FlowsToREST(items []FlowEntry) []FlowREST {
	   out := make([]FlowREST, 0, len(items))
	   for _, e := range items {
		   out = append(out, FlowToREST(e))
	  }
	   return out
   }

   func MerchantToREST(r MerchantRow) MerchantREST {
	  return MerchantREST{HoldREST: HoldToREST(r.HoldView),
		   Remaining: r.Remaining}
   }

   func MerchantsToREST(items []MerchantRow) []MerchantREST {
	   out := make([]MerchantREST, 0, len(items))
	  for _, r := range items {
		  out = append(out,
			  MerchantToREST(r))
	   }
	  return out
   }

  func SummaryToREST(s OccupancySummary) SummaryREST {
	   by := s.ByAsset
	   if by == nil {
		  by = map[string]string{}
	   }
	  return SummaryREST{
		  UserID:    s.UserID,
		  OpenHolds: s.OpenHolds, FrozenTotal: s.FrozenTotal, ByCurrency: by,
		   SellOpen: s.SellOpen, SellTotal: s.SellTotal, MerchantOpen: s.MerchantOpen,
		  MerchantTotal: s.MerchantTotal, ReleasedCount: s.ReleasedCount,
		  FlowCount: s.FlowCount, IntegrityOk: s.IntegrityOK,
	  }
  }

   func restBook(book string) string {
	   switch book {
	   case "", "funding", "spot", "main":
		   return "MAIN"
	   case "trading":
		  return "TRADE"
	   case "margin":
		  return "MARGIN"
	  default:
		  return book
	  }
  }

// ContractRoutes 读面路径登记（运维/目录）。
  func ContractRoutes() []map[string]string {
	  return []map[string]string{
		   {"method": "GET", "assets": "/v1/assets/p2p/holds", "native": "/v1/p2p/holds", "public": "/api/v1/p2p/holds"},
		  {"method": "GET", "assets": "/v1/assets/p2p/summary", "native": "/v1/p2p/summary", "public": "/api/v1/p2p/summary"},
		   {"method": "GET", "assets": "/v1/assets/p2p/details", "native": "/v1/p2p/details", "public": "/api/v1/p2p/details"},
		   {"method": "GET", "assets": "/v1/assets/p2p/holds/:id", "native": "/v1/p2p/holds/:id", "public": "/api/v1/p2p/holds/:id"},
		  {"method": "GET", "assets": "/v1/assets/p2p/flow", "native": "/v1/p2p/flow", "public": "/api/v1/p2p/flow"},
		  {"method": "GET", "assets": "/v1/assets/p2p/merchant", "native": "/v1/p2p/merchant", "public": "/api/v1/p2p/merchant"},
		   {"method": "GET", "assets": "/v1/assets/p2p/merchant/summary", "native": "/v1/p2p/merchant/summary", "public": "/api/v1/p2p/merchant/summary"},
		   {"method": "GET", "assets": "/v1/assets/p2p/orders", "native": "/v1/p2p/orders", "public": "/api/v1/p2p/orders"},
		  {"method": "GET", "assets": "/v1/assets/p2p/integrity", "native": "/v1/p2p/integrity", "public": "/api/v1/p2p/integrity"},
		  {"method": "GET", "assets": "/v1/assets/p2p/aggregate", "native": "/v1/p2p/aggregate", "public": "/api/v1/p2p/aggregate"},
		  {"method": "GET", "assets": "/v1/assets/p2p/windows", "native": "/v1/p2p/windows", "public": "/api/v1/p2p/windows"},
		  {"method": "GET", "assets": "/v1/assets/p2p/catalog", "native": "/v1/p2p/catalog", "public": "/api/v1/p2p/catalog"},
	   }
   }
