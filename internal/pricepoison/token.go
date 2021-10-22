package pricepoison

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// GenToken 代次令牌：资金写路径消费者凭此向 writegate 出示。

type GenToken struct {
	// Pair 交易对（已规范化）。
	Pair string `json:"pair"`
	// Generation 代次（单调递增；Revoke 后旧代次作废）。
	Generation uint64 `json:"generation"`
	// TickDigest tick 内容摘要 SHA-256(pair‖price‖at_ns)，十六进制。
	TickDigest string `json:"tick_digest"`
	// IssuedAt 签发时刻（UTC）。
	IssuedAt time.Time `json:"issued_at"`
	// Scope 令牌授权的写作用域（credit / occupancy / liquidation_trigger / *）。
	Scope string `json:"scope"`
}

// pairEntry 单个交易对的令牌状态。
type pairEntry struct {
	mu sync.Mutex
	generation uint64
	revoked bool // true = 当前代次已被撤销，新 tick 到来前不签发
}

// TokenRegistry 代次令牌注册表。
type TokenRegistry struct {
	mu    sync.Mutex
	pairs map[string]*pairEntry
	maxAge time.Duration
	clock func() time.Time
}

// NewTokenRegistry 构造令牌注册表。maxAge <= 0 时使用默认 60 秒。
   func NewTokenRegistry(maxAge time.Duration, clock func() time.Time) *TokenRegistry {
	   if maxAge <= 0 {
		  maxAge = 60 * time.Second
	  }
	  if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &TokenRegistry{
		pairs: map[string]*pairEntry{},
		maxAge: maxAge,
		clock: clock,
	}
}

   func (r *TokenRegistry) entry(pair string) *pairEntry {
	   r.mu.Lock()
	   e, ok := r.pairs[pair]
	   if !ok {
		   e = &pairEntry{}
		  r.pairs[pair] = e
	  }
	  r.mu.Unlock()
	  return e
   }

// Issue 为干净 tick 签发代次令牌。
// grade 必须为 GradeOK；否则返回 ErrPoisonedTick。
// scope 为允许写入的作用域名称（credit / occupancy / liquidation_trigger / *）。

  func (r *TokenRegistry) Issue(pair, tickPrice string, tickAt time.Time, grade PoisonGrade, scope string) (GenToken, error) {
	  if r == nil {
		   return GenToken{}, ErrRegistryUnavailable
	  }
	  if grade.IsPoison() {
		   return GenToken{}, fmt.Errorf("%w: grade=%s pair=%s", ErrPoisonedTick, grade.Label(), pair)
	  }
	   pair = normPair(pair)
	   scope = strings.TrimSpace(scope)
	  if scope == "" {
		  scope = "*"
	   }
	  e := r.entry(pair)
	   e.mu.Lock()
	   e.generation++
	  e.revoked = false
	  gen := e.generation
	  e.mu.Unlock()
	   digest := tickDigest(pair,
		  tickPrice, tickAt)
	   return GenToken{
		  Pair: pair,
		   Generation: gen,
		  TickDigest: digest,
		  IssuedAt:   r.clock(),
		   Scope:      scope,
	   }, nil
   }

// Revoke 撤销指定交易对当前所有活跃令牌。
// 返回被撤销的代次（可记录到 PoisonEvent.GenerationRevoked）。

  func (r *TokenRegistry) Revoke(pair string) uint64 {
	   if r == nil {
		  return 0
	   }
	  pair = normPair(pair)
	  e := r.entry(pair)
	   e.mu.Lock()
	   prev := e.generation
	  e.generation++ // 新代次使旧令牌立即失效
	  e.revoked = true
	   e.mu.Unlock()
	  return prev
  }

// Validate 验证令牌有效性：
// 1. 令牌的代次等于注册表当前代次（未被撤销覆盖）。
// 2. 注册表当前未处于 revoked 状态。
// 3. 令牌未超过 maxAge（由签发时刻到当前时钟计算）。
   func (r *TokenRegistry) Validate(tok GenToken) error {
	   if r == nil {
		   return ErrRegistryUnavailable
	   }
	  pair := normPair(tok.Pair)
	  e := r.entry(pair)
	   e.mu.Lock()
	   gen := e.generation
	  rev := e.revoked
	   e.mu.Unlock()
	  if rev || tok.Generation != gen {
		   return fmt.Errorf("%w: pair=%s token_gen=%d current_gen=%d",
			   ErrTokenRevoked, pair, tok.Generation, gen)
	  }
	  age := r.clock().Sub(tok.IssuedAt)
	  if age > r.maxAge {
		  return fmt.Errorf("%w: age=%s max=%s pair=%s",
			   ErrTokenExpired, age.String(), r.maxAge.String(), pair)
	  }
	  return nil
  }

// CurrentGeneration 返回指定交易对当前代次（调试/审计用）。
  func (r *TokenRegistry) CurrentGeneration(pair string) uint64 {
	   if r == nil {
		  return 0
	   }
	   pair = normPair(pair)
	  e := r.entry(pair)
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  return e.generation
  }

// IsRevoked 返回指定交易对是否处于撤销状态。
  func (r *TokenRegistry) IsRevoked(pair string) bool {
	  if r == nil {
		  return true
	   }
	  pair = normPair(pair)
	   e := r.entry(pair)
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   return e.revoked
  }

// normPair 规范化交易对名称（大写 + 去空白）。
  func normPair(pair string) string {
	  return strings.ToUpper(strings.TrimSpace(pair))
   }

// ExportTickDigest 计算并返回 tick 内容摘要（供 writegate 包使用）。
// 等价于内部 tickDigest，算法：SHA-256(pair‖"\x00"‖price‖"\x00"‖at_unix_ns)。

   func ExportTickDigest(pair, price string, at time.Time) string {
	   return tickDigest(pair, price, at)
  }

// tickDigest 计算 tick 内容摘要：SHA-256(pair‖"\x00"‖price‖"\x00"‖at_unix_ns)。
   func tickDigest(pair, price string, at time.Time) string {
	  h := sha256.New()
	  sep := []byte{0x00}
	  _, _ = h.Write([]byte(pair))
	   _, _ = h.Write(sep)
	   _, _ = h.Write([]byte(price))
	   _, _ = h.Write(sep)
	  _, _ = h.Write([]byte(fmt.Sprintf("%d", at.UnixNano())))
	   return hex.EncodeToString(h.Sum(nil))
   }
