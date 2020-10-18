// 功能：充值入账域之匹配入账。
// 作者：Draven
// 日期：2020.10.17

package deposit

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type MatchStatus string

const (
	MatchExact   MatchStatus = "exact"
	MatchPending MatchStatus = "pending_confirms"
	MatchPartial MatchStatus = "amount_mismatch"
	MatchUnknown MatchStatus = "unbound_address"
	MatchAsset MatchStatus = "asset_mismatch"
	MatchReorg  MatchStatus = "reorged"
	MatchReject MatchStatus = "rejected"
)

type Candidate struct {
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Address       string `json:"address"`
	Confirmations int    `json:"confirmations"`
	Need          int    `json:"need"`
	Status MatchStatus `json:"status"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
}

type Matcher struct {
	mu sync.Mutex
	pool *addrpool.Pool
	need map[string]int
	seen  map[string]Candidate
	clock func() time.Time
}

   func NewMatcher(pool *addrpool.Pool) *Matcher {
	  if pool == nil {
		   pool = addrpool.New(wallet.DeriveAddress, 0)
	   }
	   return &Matcher{
		   pool: pool,
		  need: map[string]int{"ETH": 12, "BTC": 3, "TRON": 20, "SOL": 32, "BASE": 20, "POLYGON": 20, "ARBITRUM": 20, "OPTIMISM": 20},
		   seen: map[string]Candidate{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

   func (m *Matcher) Need(chain string, n int) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.need[strings.ToUpper(strings.TrimSpace(chain))] = n
   }

  func confirmNeed(m *Matcher, chain string) int {
	  if n, ok := m.need[chain]; ok && n > 0 {
		   return n
	   }
	   return 12
  }

// Match 把观察结果对到地址池。
// 地址未绑定 → unbound，不得入账。资产不对 → asset_mismatch。
// 金额对不上 → amount_mismatch（调用方不得用请求额抹平）。
// 确认数不够 → pending_confirms。重组 → reorged。
  func (m *Matcher) Match(obs wallet.Observation, expectAsset, expectAmount string) (Candidate, error) {
	   if m == nil {
		   return Candidate{}, fmt.Errorf("deposit matcher unavailable")
	   }
	   chain, err := validate.Chain(guessChain(obs))
	   if err != nil {
		   chain = strings.ToUpper(strings.TrimSpace(obs.Hash))
		   if len(chain) > 16 {
			  chain = "ETH"
		  }
	   }
	  if obs.Hash == "" {
		  return Candidate{}, validate.ErrTxHash
	   }
	  addr := strings.TrimSpace(obs.ToAddress)
	   c := Candidate{
		   Chain: chain, TxHash: obs.Hash, Asset: strings.ToUpper(strings.TrimSpace(obs.Asset)),
		  Amount: obs.Amount.String(), Address: addr, Confirmations: obs.Confirmations,
		  At: m.clock(),
	   }
	  bind, ok := m.pool.ByAddress(addr)
	  if ok && bind.Allocated {
		  c.UserID = bind.UserID
	  }
	   if obs.Reorged || obs.Status == wallet.TxReorged {
		  c.Status = MatchReorg
		  c.Reason = "observation reorged"
		  m.put(c)
		  return c, nil
	  }
	  if !ok || !bind.Allocated {
		   c.Status = MatchUnknown
		  c.Reason = "address not in allocation pool"
		   m.put(c)
		   return c, nil
	   }
	  c.UserID = bind.UserID
	   c.Need = confirmNeed(m, chain)
	   if expectAsset != "" && c.Asset != "" && !strings.EqualFold(c.Asset, expectAsset) {
		  c.Status = MatchAsset
		  c.Reason = "observed asset " + c.Asset
		  m.put(c)
		  return c, nil
	  }
	  if strings.TrimSpace(expectAmount) != "" {
		   want, err := money.Parse(expectAmount)
		   if err != nil {
			   if want, err = money.ParseBook(expectAmount); err != nil {
				   return Candidate{}, err
			  }
		  }
		   got, err := money.ParseBook(c.Amount)
		   if err != nil {
			  return Candidate{}, err
		   }
		   if !got.Equal(want) {
			   c.Status = MatchPartial
			   c.Reason = "observed " + got.String() + " expected " + want.String()
			  m.put(c)
			   return c, nil
		   }
	   }
	  if c.Confirmations < c.Need {
		   c.Status = MatchPending
		   c.Reason = fmt.Sprintf("confirms %d/%d", c.Confirmations, c.Need)
		  m.put(c)
		   return c, nil
	   }
	  c.Status = MatchExact
	   m.put(c)
	  return c, nil
   }

  func (m *Matcher) put(c Candidate) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  m.seen[c.Chain+"|"+c.TxHash] = c
   }

  func (m *Matcher) Get(chain, hash string) (Candidate, bool) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   c, ok := m.seen[strings.ToUpper(strings.TrimSpace(chain))+"|"+strings.TrimSpace(hash)]
	   return c, ok
   }

  func (m *Matcher) Pending() []Candidate {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]Candidate, 0)
	  for _, c := range m.seen {
		  if c.Status == MatchPending {
			  out = append(out, c)
		  }
	   }
	  return out
  }

   func (m *Matcher) CanCredit(c Candidate) bool { return c.Status == MatchExact }

   func guessChain(obs wallet.Observation) string {
	  if strings.HasPrefix(strings.ToLower(obs.Hash), "0x") {
		   return "ETH"
	  }
	  return "BTC"
   }

  func (s *Service) Matcher() *Matcher {
	  if s == nil {
		   return nil
	  }
	  if s.match == nil {
		   s.match = NewMatcher(nil)
	  }
	   return s.match
  }

// OpenExceptionFromMatch 匹配漂移落异常单（含 Asset）。
// 仅 amount/asset/reorg/memo 类；unbound 不落单。
// 空 user 跳过；actor 固定 deposit-match。
   func (s *Service) OpenExceptionFromMatch(c Candidate) (Exception, error) {
	   if s == nil || s.desk == nil {
		   return Exception{}, ErrTicketMiss
	   }
	  if strings.TrimSpace(c.UserID) == "" {
		  return Exception{}, ErrTicketMiss
	   }
	   var kind TicketKind
	  switch c.Status {
	   case MatchPartial:
		   kind = TicketAmtDrift
	  case MatchAsset:
		  kind = TicketHashDrift
	   case MatchReorg:
		   kind = TicketReorgHold
	  default:
		  return Exception{}, ErrTicketKind
	  }
	   note := c.Reason
	  if note == "" {
		  note = string(c.Status)
	  }
	   return s.desk.OpenException("deposit-match", c.UserID, c.Chain, c.Asset, c.TxHash, string(kind), note)
   }
