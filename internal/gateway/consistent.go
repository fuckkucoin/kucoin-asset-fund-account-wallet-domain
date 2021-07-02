package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Snap struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Book   string `json:"book"`
	Avail  string `json:"available"`
	Frozen string `json:"frozen"`
	Seq    int64  `json:"journal_seq"`
	Fence persist.Fence `json:"fence"`
}

// Reader 读路径一致性：围栏 token 对不上则拒，不得用过期快照出账。

type Reader struct {
	mu    sync.Mutex
	fence *persist.Table
	snaps map[string]Snap
}

   func NewReader() *Reader {
	   return &Reader{fence: persist.NewTable(), snaps: map[string]Snap{}}
   }

  func snapKey(userID, book, asset string) string {
	  return userID + "|" + book + "|" + asset
  }

  func (r *Reader) Put(userID, book, asset, avail, frozen string, seq int64) (Snap, error) {
	   if r == nil {
		   return Snap{}, ErrUnavailable
	  }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		  return Snap{}, ErrUser
	  }
	  as, err := validate.Asset(asset)
	   if err != nil {
		  return Snap{}, ErrAsset
	   }
	  bk, err := validate.Book(book)
	  if err != nil {
		   return Snap{}, err
	  }
	   if _, err := money.ParseBook(avail); err != nil {
		  return Snap{}, err
	   }
	  if _, err := money.ParseBook(frozen); err != nil {
		   return Snap{}, err
	  }
	  k := snapKey(uid, bk, as)
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   if old, ok := r.snaps[k]; ok && seq <= old.Seq {
		  return Snap{}, fmt.Errorf("asset snapshot seq not monotonic")
	   }
	  f,
		   err := r.fence.Bump(k, persist.NewToken())
	  if err != nil {
		   return Snap{}, err
	   }
	  s := Snap{UserID: uid, Asset: as, Book: bk, Avail: avail, Frozen: frozen, Seq: seq, Fence: f}
	  r.snaps[k] = s
	   return s, nil
  }

  func (r *Reader) Get(userID, book, asset string, epoch int64, token string) (Snap, error) {
	   if r == nil {
		   return Snap{}, ErrUnavailable
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return Snap{}, ErrUser
	   }
	  as, err := validate.Asset(asset)
	  if err != nil {
		  return Snap{}, ErrAsset
	   }
	  bk, err := validate.Book(book)
	   if err != nil {
		   return Snap{}, err
	   }
	   k := snapKey(uid, bk, as)
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  if err := r.fence.CompareAndHold(k, epoch, token); err != nil {
		  return Snap{}, err
	  }
	   s, ok := r.snaps[k]
	  if !ok {
		   return Snap{}, fmt.Errorf("asset snapshot missing")
	  }
	  if s.Seq < 0 {
		   return Snap{}, fmt.Errorf("asset snapshot seq invalid")
	   }
	   return s, nil
  }

  func (r *Reader) RequireFresh(_ context.Context, want, got Snap) error {
	  if want.UserID != got.UserID || want.Asset != got.Asset || want.Book != got.Book {
		  return fmt.Errorf("asset snapshot identity drift")
	  }
	  if want.Seq != got.Seq || !want.Fence.Match(got.Fence.Epoch, got.Fence.Token) {
		   return persist.ErrStaleFence
	  }
	  if strings.TrimSpace(want.Avail) != strings.TrimSpace(got.Avail) {
		   return fmt.Errorf("asset snapshot available drift")
	   }
	  return nil
   }
