package brokerfence

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

const defaultTokenTTL = 10 * time.Minute

// MutexStore 互斥令牌仓库（内存），每个子账最多一个活跃令牌。
// 部署可替换为 Redis SETNX，此处以内存实现满足集成与单机模式。

type MutexStore struct {
	mu sync.Mutex
	tokens map[string]*MutexToken // key=subID
	ttl   time.Duration
	clock func() time.Time
}

// NewMutexStore 构造令牌仓库，ttl<=0 时用默认 10 分钟。

   func NewMutexStore(ttl time.Duration) *MutexStore {
	   if ttl <= 0 {
		   ttl = defaultTokenTTL
	   }
	   return &MutexStore{
		   tokens: make(map[string]*MutexToken),
		   ttl:    ttl,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (ms *MutexStore) now() time.Time {
	   if ms.clock != nil {
		  return ms.clock()
	  }
	  return time.Now().UTC()
   }

// Issue 颁发互斥令牌。若已有令牌且未过期，覆盖之（运维强制接管场景）。
// 返回新令牌，调用方须将 token 值传给持有方（经纪 ops）。
  func (ms *MutexStore) Issue(leaseID, subID, actor string) (MutexToken, error) {
	  leaseID = strings.TrimSpace(leaseID)
	   subID = strings.TrimSpace(subID)
	   actor = strings.TrimSpace(actor)
	   if leaseID == "" || subID == "" || actor == "" {
		   return MutexToken{}, ErrInput
	  }
	   now := ms.now()
	   tok := &MutexToken{
		   Token:   uuid.NewString(),
		  LeaseID: leaseID,
		  SubID:   subID,
		   Actor:   actor,
		   IssuedAt:  now,
		  ExpiredAt: now.Add(ms.ttl),
	  }
	   ms.mu.Lock()
	  ms.tokens[subID] = tok
	   ms.mu.Unlock()
	   return *tok, nil
  }

// Revoke 撤销指定子账的所有令牌（围栏关闭时调用）。

   func (ms *MutexStore) Revoke(subID string) {
	   subID = strings.TrimSpace(subID)
	   ms.mu.Lock()
	   delete(ms.tokens, subID)
	  ms.mu.Unlock()
  }

// ValidateToken 校验令牌是否有效（存在+未过期+token 值匹配）。
// 失败返回 ErrMutexDenied 或 ErrTokenExpired。
   func (ms *MutexStore) ValidateToken(subID, token string, now time.Time) error {
	  subID = strings.TrimSpace(subID)
	   token = strings.TrimSpace(token)
	  ms.mu.Lock()
	   tok, ok := ms.tokens[subID]
	  ms.mu.Unlock()
	  if !ok {
		   return ErrMutexDenied
	  }
	  if tok.Token != token {
		   return ErrMutexDenied
	   }
	   if now.After(tok.ExpiredAt) {
		  return ErrTokenExpired
	   }
	   return nil
  }

// Get 返回子账当前活跃令牌（若存在且未过期）。

   func (ms *MutexStore) Get(subID string) (MutexToken, bool) {
	  subID = strings.TrimSpace(subID)
	  ms.mu.Lock()
	   tok, ok := ms.tokens[subID]
	   ms.mu.Unlock()
	   if !ok {
		   return MutexToken{}, false
	  }
	  if ms.now().After(tok.ExpiredAt) {
		  return MutexToken{}, false
	   }
	  return *tok, true
   }

// PurgeExpired 清理所有已过期令牌（由 worker 定期调用）。
   func (ms *MutexStore) PurgeExpired() int {
	  now := ms.now()
	  ms.mu.Lock()
	   defer ms.mu.Unlock()
	  removed := 0
	  for k, tok := range ms.tokens {
		   if now.After(tok.ExpiredAt) {
			  delete(ms.tokens, k)
			  removed++
		   }
	  }
	   return removed
   }
