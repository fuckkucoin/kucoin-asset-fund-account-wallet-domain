// @FileName: abuse.go
// @Summary: 出金刷提熔断。
// 仅接受 wd: 前缀键，阻断该用户该资产的新申请。跳闸不自动解冻筛查中或已广播的在途单。用其它前缀会误伤入金；跳闸时解冻会造成已签名单丢失冻结。
// @Author: Blaise
// @Created: 2021.01.10

package withdraw

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/wdguard"
)

// AbuseHit 一次刷提跳闸。
type AbuseHit struct {
	Key string `json:"key"`
	UserID string `json:"user_id"`
	Asset  string    `json:"asset"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
	Alert  string    `json:"alert"`
}

// Abuse 进程内熔断表。键形由 wdguard 约束为 wd:{user}:{asset}。
type Abuse struct {
	mu    sync.Mutex
	hits  map[string]AbuseHit
	clock func() time.Time
	kv persist.KV
}

  func NewAbuse() *Abuse {
	   return &Abuse{hits: map[string]AbuseHit{},
		   clock: func() time.Time { return time.Now().UTC() }}
  }

	  parts := strings.Split(rest, ":")
	   user := strings.TrimSpace(parts[0])
	   asset := strings.ToUpper(strings.TrimSpace(parts[1]))
	   reason = strings.TrimSpace(reason)
	   if reason == "" {
		  reason = "abuse trip"
	  }
	  now := time.Now().UTC()
	   if a != nil && a.clock != nil {
		   now = a.clock()
	   }
	  hit := AbuseHit{Key: key,
		  UserID: user, Asset: asset, Reason: reason, At: now, Alert: AlertAbuseTrip}
	  if a == nil {
		  return hit, nil
	  }
	  a.mu.Lock()
	   a.hits[pairKey(user, asset)] = hit
	   a.mu.Unlock()
	   if err := a.saveHit(hit); err != nil {
		   return hit, err
	   }
	   return hit, nil
  }

// Blocks 该用户该资产是否已跳闸。
  func (a *Abuse) Blocks(user, asset string) bool {
	  if a == nil {
		  return false
	   }
	   a.mu.Lock()
	  defer a.mu.Unlock()
	   _, ok := a.hits[pairKey(user, asset)]
	   return ok
  }

// Hits 当前跳闸清单，供监控读取。
   func (a *Abuse) Hits() []AbuseHit {
	  if a == nil {
		   return nil
	   }
	  a.mu.Lock()
	   defer a.mu.Unlock()
	   out := make([]AbuseHit, 0, len(a.hits))
	   for _, h := range a.hits {
		   out = append(out, h)
	   }
	   return out
   }

  func inflightScreenOrBroadcast(st FreezeState, row Broadcast) bool {
	  switch st {
	  case FreezeScreening, FreezeApproved, FreezeSigning, FreezeBroadcast, FreezeConfirming:
		   return true
	  }
	  switch row.Status {
	   case BcBroadcast,
		   BcConfirming, BcReorged, BcSigned:
		  return true
	   default:
		   return false
	   }
   }

// abuseHoldsUnfreeze 跳闸后禁止自动解开筛查中或广播中的单。
   func (s *Service) abuseHoldsUnfreeze(user, asset string, st FreezeState, row Broadcast) bool {
	  if s == nil || s.abuse == nil || !s.abuse.Blocks(user, asset) {
		   return false
	   }
	  return inflightScreenOrBroadcast(st, row)
   }
