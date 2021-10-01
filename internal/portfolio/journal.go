package portfolio

import (
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// HopRec 科目跳转流水。同一幂等键只能绑同一组 from/to/asset/amount。

type HopRec struct {
	Key string `json:"key"`
	UserID string `json:"user_id"`
	From   Book   `json:"from"`
	To     Book   `json:"to"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Result HopResult `json:"result"`
	At    time.Time     `json:"at"`
	Fence persist.Fence `json:"fence"`
}

// HopIdem 带幂等键的 hop。键撞了但金额/方向不同直接拒，不得改已经跳过的账。
  func (l *BookLedger) HopIdem(userID string, from, to Book, asset, amount, key string) (HopResult, error) {
	  if l == nil {
		   return HopResult{}, ErrUnavailable
	   }
	  dir, err := HopDir(from, to)
	   if err != nil {
		   return HopResult{}, err
	   }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		  return HopResult{}, ErrUser
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return HopResult{}, err
	  }
	   amt, err := money.Parse(amount)
	  if err != nil {
		  return HopResult{}, err
	   }
	  key, err = validate.Idem(key)
	  if err != nil {
		  return HopResult{}, ErrIdem
	   }
	  l.mu.Lock()
	  defer l.mu.Unlock()
	   if rec, ok := l.journal[key]; ok {
		   if rec.UserID == uid && rec.From == from && rec.To == to && rec.Asset == as && rec.Amount == amt.String() {
			  return rec.Result, nil
		   }
		   return HopResult{}, ErrIdem
	  }
	   got, err := l.hopLocked(uid, from, to, as, amt, amount, dir)
	  if err != nil {
		  return HopResult{}, err
	   }
	   f, err := l.fence.Bump("hop:"+key, persist.NewToken())
	  if err != nil {
		  return HopResult{}, err
	   }
	  cp := HopRec{
		  Key: key, UserID: uid, From: from, To: to, Asset: as, Amount: amt.String(),
		  Result: got, At: l.clock(), Fence: f,
	   }
	  if l.journal == nil {
		  l.journal = map[string]*HopRec{}
	   }
	  l.journal[key] = &cp
	  return got, nil
   }

   func (l *BookLedger) Journal(userID string) []HopRec {
	   if l == nil {
		   return nil
	  }
	   l.mu.Lock()
	   defer l.mu.Unlock()
	  out := make([]HopRec, 0, len(l.journal))
	   for _, rec := range l.journal {
		  if userID != "" && rec.UserID != userID {
			   continue
		   }
		   out = append(out, *rec)
	  }
	   return out
   }

   func (l *BookLedger) JournalGet(key string) (HopRec, bool) {
	   if l == nil {
		  return HopRec{}, false
	   }
	   l.mu.Lock()
	  defer l.mu.Unlock()
	   rec, ok := l.journal[key]
	  if !ok {
		  return HopRec{}, false
	   }
	   return *rec, true
   }
