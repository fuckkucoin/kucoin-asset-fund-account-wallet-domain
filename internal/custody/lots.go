// @FileName: lots.go
// @Summary: 托管与多方签发域之托管批次。
// @Author: Jensen
// @Created: 2021.01.20

package custody

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Lot struct {
	ID    string `json:"id"`
	Asset string `json:"asset"`
	Side     string    `json:"side"` // hot / cold
	Amount   string    `json:"amount"`
	Remain   string    `json:"remain"`
	OpenedAt time.Time `json:"opened_at"`
	Closed bool `json:"closed"`
}

type Movement struct {
	ID    string `json:"id"`
	Asset string `json:"asset"`
	From string `json:"from"`
	To   string `json:"to"`
	Amount string `json:"amount"`
	Kind   string `json:"kind"`
	LotID string    `json:"lot_id,omitempty"`
	At    time.Time `json:"at"`
}

// Ledger 托管库存流水。热/冷进出按 FIFO 耗 lot；合计必须守恒。
type Ledger struct {
	mu sync.Mutex
	lots  []*Lot
	moves []Movement
	clock func() time.Time
}

  func NewLedger() *Ledger {
	   return &Ledger{clock: func() time.Time { return time.Now().UTC() }}
   }

  func (l *Ledger) Receive(side, asset, amount, kind string) (Lot, Movement, error) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  amt, err := money.Parse(amount)
	  if err != nil {
		  return Lot{}, Movement{}, err
	  }
	   side = normalizeSide(side)
	  if side == "" {
		  return Lot{}, Movement{}, fmt.Errorf("custody side required")
	   }
	  if kind == "" {
		   kind = "receive"
	   }
	   now := l.clock()
	   lot := Lot{ID: uuid.NewString(), Asset: asset, Side: side, Amount: amt.String(), Remain: amt.String(), OpenedAt: now}
	  mv := Movement{ID: uuid.NewString(), Asset: asset, To: side, Amount: amt.String(), Kind: kind, LotID: lot.ID, At: now}
	  l.mu.Lock()
	   defer l.mu.Unlock()
	  cp := lot
	   l.lots = append(l.lots, &cp)
	   l.moves = append(l.moves, mv)
	  return lot, mv, nil
   }

// Spend FIFO 从 side 扣 remain。不够拒，不得把 lot 打负。

  func (l *Ledger) Spend(side, asset, amount, kind string) ([]Movement, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   need, err := money.Parse(amount)
	   if err != nil {
		  return nil, err
	  }
	   side = normalizeSide(side)
	   if kind == "" {
		  kind = "spend"
	   }
	   l.mu.Lock()
	  defer l.mu.Unlock()
	  left := need
	   out := make([]Movement, 0)
	  now := l.clock()
	   for _, lot := range l.lots {
		   if left.IsZero() {
			  break
		  }
		   if lot.Closed || lot.Asset != asset || lot.Side != side {
			  continue
		   }
		   rem, err := money.ParseBook(lot.Remain)
		  if err != nil {
			   return nil, err
		  }
		   if !rem.IsPositive() {
			  lot.Closed = true
			  continue
		   }
		   take := rem
		   if rem.Cmp(left) > 0 {
			  take = left
		  }
		  next, err := rem.Sub(take)
		   if err != nil {
			  return nil, err
		   }
		  lot.Remain = next.String()
		   if next.IsZero() {
			  lot.Closed = true
		   }
		  left, err = left.Sub(take)
		   if err != nil {
			   return nil, err
		   }
		  mv := Movement{ID: uuid.NewString(), Asset: asset, From: side, Amount: take.String(), Kind: kind, LotID: lot.ID, At: now}
		   l.moves = append(l.moves, mv)
		  out = append(out, mv)
	   }
	  if left.IsPositive() {
		   return nil, fmt.Errorf("insufficient %s inventory on %s", asset, side)
	   }
	  return out, nil
  }

// Transfer 热→冷或冷→热。同一把锁里 spend+receive，失败整笔不落半截。
   func (l *Ledger) Transfer(from, to, asset, amount string) error {
	  from, to = normalizeSide(from), normalizeSide(to)
	   if from == "" || to == "" || from == to {
		  return fmt.Errorf("custody transfer sides")
	  }
	   if _, err := money.Parse(amount); err != nil {
		   return err
	   }
	   l.mu.Lock()
	  defer l.mu.Unlock()
	  if _, err := l.spendInner(from, asset, amount, "transfer_out"); err != nil {
		  return err
	   }
	   _, _, err := l.receiveInner(to, asset, amount, "transfer_in")
	   return err
   }

   func (l *Ledger) spendInner(side, asset, amount, kind string) ([]Movement, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   need, err := money.Parse(amount)
	  if err != nil {
		  return nil, err
	   }
	   side = normalizeSide(side)
	  left := need
	   out := make([]Movement, 0)
	   now := l.clock()
	  for _, lot := range l.lots {
		   if left.IsZero() {
			   break
		  }
		  if lot.Closed || lot.Asset != asset || lot.Side != side {
			  continue
		   }
		  rem, err := money.ParseBook(lot.Remain)
		  if err != nil {
			  return nil, err
		   }
		   if !rem.IsPositive() {
			  lot.Closed = true
			   continue
		   }
		   take := rem
		  if rem.Cmp(left) > 0 {
			  take = left
		   }
		   next, err := rem.Sub(take)
		   if err != nil {
			   return nil, err
		   }
		  lot.Remain = next.String()
		   if next.IsZero() {
			  lot.Closed = true
		   }
		  left, err = left.Sub(take)
		  if err != nil {
			   return nil, err
		  }
		   mv := Movement{ID: uuid.NewString(), Asset: asset, From: side, Amount: take.String(), Kind: kind, LotID: lot.ID, At: now}
		  l.moves = append(l.moves, mv)
		   out = append(out, mv)
	  }
	   if left.IsPositive() {
		  return nil, fmt.Errorf("insufficient %s inventory on %s", asset, side)
	  }
	  return out, nil
  }

  func (l *Ledger) receiveInner(side, asset, amount, kind string) (Lot, Movement, error) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   amt, err := money.Parse(amount)
	   if err != nil {
		  return Lot{}, Movement{}, err
	  }
	   side = normalizeSide(side)
	  now := l.clock()
	  lot := Lot{ID: uuid.NewString(), Asset: asset, Side: side, Amount: amt.String(), Remain: amt.String(), OpenedAt: now}
	  mv := Movement{ID: uuid.NewString(),
		   Asset: asset, To: side, Amount: amt.String(), Kind: kind, LotID: lot.ID, At: now}
	   cp := lot
	  l.lots = append(l.lots, &cp)
	   l.moves = append(l.moves, mv)
	  return lot, mv, nil
  }

   func (l *Ledger) Remain(side, asset string) (string, error) {
	  l.mu.Lock()
	  defer l.mu.Unlock()
	   side = normalizeSide(side)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   acc := money.Zero()
	  for _, lot := range l.lots {
		   if lot.Asset != asset || lot.Side != side || lot.Closed {
			  continue
		  }
		   rem, err := money.ParseBook(lot.Remain)
		   if err != nil {
			  return "0", err
		   }
		   acc, err = acc.Add(rem)
		  if err != nil {
			  return "0", err
		   }
	  }
	   return acc.String(), nil
   }

   func (l *Ledger) Movements(limit int) []Movement {
	  l.mu.Lock()
	   defer l.mu.Unlock()
	  if limit <= 0 || limit > len(l.moves) {
		  limit = len(l.moves)
	   }
	   out := make([]Movement, limit)
	   copy(out, l.moves[len(l.moves)-limit:])
	  return out
  }

   func (l *Ledger) Lots(side, asset string) []Lot {
	   l.mu.Lock()
	   defer l.mu.Unlock()
	  out := make([]Lot, 0)
	  for _, lot := range l.lots {
		  if side != "" && lot.Side != normalizeSide(side) {
			   continue
		  }
		   if asset != "" && lot.Asset != strings.ToUpper(asset) {
			   continue
		  }
		   out = append(out, *lot)
	  }
	  return out
  }

   func normalizeSide(s string) string {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	   case "hot", "bridge":
		  return "hot"
	  case "cold":
		   return "cold"
	   default:
		  return ""
	  }
  }
