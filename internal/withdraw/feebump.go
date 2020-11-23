// 出金提现域之手续费抬升。

// Author: Bronson
// Date: 2020.10.29

package withdraw

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
)

type FeeBump struct {
	TransferID string `json:"transfer_id"`
	Chain   string `json:"chain"`
	OldHash string `json:"old_hash"`
	NewHash string `json:"new_hash,omitempty"`
	OldFee string `json:"old_fee"`
	NewFee   string    `json:"new_fee"`
	Replaced bool      `json:"replaced"`
	At       time.Time `json:"at"`
	Memo string `json:"memo,omitempty"`
	Tag     string `json:"tag,omitempty"`
	TagName string `json:"tag_name,omitempty"`
	DestinationTag string `json:"destination_tag,omitempty"` // 抬价独立编解码面透传
}

type Bumper struct {
	mu   sync.Mutex
	rows map[string]*FeeBump
	clock          func() time.Time
	sql            *persist.SQLDB
	lastPersistErr string
	hydrateErr string
	hydratedAt time.Time
	hydratedN int
}

   func NewBumper() *Bumper {
	  return &Bumper{rows: map[string]*FeeBump{}, clock: func() time.Time { return time.Now().UTC() }}
   }

// BindSQL 抬价落库；OpenMem 不绑。

  func (b *Bumper) BindSQL(db *persist.SQLDB) *Bumper {
	  if b != nil {
		  b.sql = db
	   }
	  return b
   }

  func (b *Bumper) sqlOn() bool {
	  return b != nil && b.sql != nil && b.sql.DB != nil
  }

// Hydrate 启动从 SQL 回填抬价，跨重启 By-ID feeBump 可观测。
// Open 后须 Hydrate；失败不得清空内存。
// Hydrate 失败时抬价面空，不得伪造链上替换。
  func (b *Bumper) Hydrate(ctx context.Context) error {
	   if b == nil || !b.sqlOn() {
		   return nil
	  }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   list, err := sqlrepo.NewWDTrack(b.sql).ListFeeBumps(ctx)
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   if err != nil {
		   b.hydrateErr = err.Error()
		  return err
	  }
	   if b.rows == nil {
		   b.rows = map[string]*FeeBump{}
	   }
	  for _, row := range list {
		   cp := FeeBump{
			   TransferID: row.TransferID, Chain: row.Chain, OldHash: row.OldHash, NewHash: row.NewHash,
			   OldFee: row.OldFee, NewFee: row.NewFee, Replaced: row.Replaced, At: row.BumpedAt,
			  Memo: row.Memo,
			   Tag:  row.Tag, TagName: row.TagName, DestinationTag: row.DestinationTag,
		  }
		   b.rows[row.TransferID] = &cp
	  }
	   b.hydratedN = len(list)
	  b.hydratedAt = b.clock()
	   b.hydrateErr = ""
	  return nil
   }

// persistLocked 持锁落库抬价；错误写入 lastPersistErr。

   func (b *Bumper) persistLocked(row FeeBump) error {
	   if !b.sqlOn() {
		  return nil
	  }
	   err := sqlrepo.NewWDTrack(b.sql).PutFeeBump(context.Background(), sqlrepo.WDFeeBump{
		  TransferID: row.TransferID, Chain: row.Chain, OldHash: row.OldHash, NewHash: row.NewHash,
		  OldFee: row.OldFee, NewFee: row.NewFee, Replaced: row.Replaced, BumpedAt: row.At,
		  Memo: row.Memo, Tag: row.Tag, TagName: row.TagName, DestinationTag: row.DestinationTag,
	   })
	  if err != nil {
		   b.lastPersistErr = err.Error()
		   return fmt.Errorf("fee bump persist: %w", err)
	  }
	   b.lastPersistErr = ""
	  return nil
   }

// Health 抬价跟踪健康面。

  func (b *Bumper) Health() TrackHealth {
	  if b == nil {
		  return TrackHealth{}
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   return TrackHealth{
		   SQLBound: b.sqlOn(), HydratedN: b.hydratedN, HydratedAt: b.hydratedAt,
		  HydrateErr: b.hydrateErr,
		   MemoryN: len(b.rows), LastPersistErr: b.lastPersistErr,
	   }
  }

// Bump 提高未确认出金手续费。新费必须严格大于旧费；金额守恒由调用方另核本金。

  func (b *Bumper) Bump(trID, chain, oldHash, oldFee, newFee, newHash string) (FeeBump, error) {
	  if b == nil {
		   return FeeBump{}, fmt.Errorf("fee bumper unavailable")
	  }
	  if strings.TrimSpace(trID) == "" {
		   return FeeBump{}, fmt.Errorf("transfer_id required")
	  }
	  if _, err := validate.Chain(chain); err != nil {
		  return FeeBump{}, err
	   }
	  oldA,
		   err := money.ParseBook(orZ(oldFee))
	  if err != nil {
		  return FeeBump{}, err
	   }
	   newA, err := money.Parse(newFee)
	  if err != nil {
		   return FeeBump{}, err
	  }
	  if newA.Cmp(oldA) <= 0 {
		  return FeeBump{}, fmt.Errorf("fee bump must increase")
	  }
	   row := FeeBump{
		   TransferID: trID, Chain: strings.ToUpper(chain), OldHash: oldHash,
		  NewHash: newHash, OldFee: oldA.String(), NewFee: newA.String(),
		   Replaced: newHash != "" && newHash != oldHash, At: b.clock(),
	  }
	   b.mu.Lock()
	  b.rows[trID] = &row
	  err = b.persistLocked(row)
	  b.mu.Unlock()
	   if err != nil {
		  return row, err
	   }
	   return row, nil
  }

  func (b *Bumper) Get(trID string) (FeeBump, bool) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   r, ok := b.rows[trID]
	   if !ok {
		   return FeeBump{}, false
	  }
	   return *r, true
  }

   func orZ(s string) string {
	   if strings.TrimSpace(s) == "" {
		  return "0"
	  }
	  return s
   }

  func (s *Service) Bumper() *Bumper {
	  if s == nil {
		   return NewBumper()
	   }
	   if s.bump == nil {
		  s.bump = NewBumper()
	   }
	   return s.bump
  }
