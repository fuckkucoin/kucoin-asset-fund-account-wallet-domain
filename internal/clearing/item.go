package clearing

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusReady Status = "ready"
	StatusReleased Status = "released"
	StatusExcepted Status = "excepted"
	StatusClosed   Status = "closed"
)

type Item struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
	UserID  string `json:"user_id"`
	Asset   string `json:"asset"`
	Amount     string `json:"amount"`
	MovementID string `json:"movement_id,omitempty"`
	Offset int `json:"offset"`
	TradeAt   time.Time `json:"trade_at"`
	ValueDate time.Time `json:"value_date"`
	Status    Status    `json:"status"`
	Note      string    `json:"note,omitempty"`
	Fence persist.Fence `json:"fence"`
}

  func (b *Book) Add(batchID, userID, asset, amount, movement string, offset int, trade time.Time, skipWeekend bool) (Item, error) {
	   if b == nil {
		  return Item{}, ErrUnavailable
	   }
	   if _, err := ParseOffset(offset); err != nil {
		  return Item{}, err
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return Item{}, err
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return Item{}, err
	  }
	   if _, err := money.Parse(amount); err != nil {
		  return Item{}, err
	   }
	  if movement != "" {
		   if _, err := validate.MovementID(movement); err != nil {
			   return Item{}, err
		   }
	  }
	  vd, err := ValueDateCal(trade, offset, skipWeekend, b.hol)
	  if err != nil {
		   return Item{}, err
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  bat, ok := b.batches[strings.TrimSpace(batchID)]
	  if !ok {
		  return Item{}, ErrNotFound
	  }
	   if bat.Status == BatchClosed {
		   return Item{}, ErrClosed
	  }
	   it := Item{
		   ID: uuid.NewString(), BatchID: bat.ID, UserID: uid, Asset: as, Amount: amount,
		   MovementID: strings.TrimSpace(movement), Offset: offset, TradeAt: trade.UTC(),
		  ValueDate: vd, Status: StatusPending,
	  }
	   f, err := b.fence.Bump(it.ID, persist.NewToken())
	   if err != nil {
		   return Item{}, err
	  }
	   it.Fence = f
	  cp := it
	  b.items[it.ID] = &cp
	   b.byBatch[bat.ID] = append(b.byBatch[bat.ID], it.ID)
	  return it, nil
   }

   func (b *Book) Item(id string) (Item,
	  error) {
	  if b == nil {
		  return Item{}, ErrUnavailable
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  it, ok := b.items[strings.TrimSpace(id)]
	   if !ok {
		  return Item{}, ErrNotFound
	   }
	  return *it, nil
   }

   func (b *Book) Items(batchID string) []Item {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   ids := b.byBatch[strings.TrimSpace(batchID)]
	   out := make([]Item, 0, len(ids))
	   for _, id := range ids {
		  if it := b.items[id]; it != nil {
			   out = append(out, *it)
		  }
	  }
	  return out
  }
