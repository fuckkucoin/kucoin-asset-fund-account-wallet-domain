package fundcircuit

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// defaultLedgerCap 环形表上限。超出后丢最旧，避免刷量把内存吃光。
const defaultLedgerCap = 4096

// Ledger 滥用信号台账。写入按时间追加，读取按作用域键过滤。

type Ledger struct {
	mu   sync.Mutex
	rows []AbuseSignal
	cap  int
	ttl   time.Duration
	clock func() time.Time
	ingest  int64
	dropped int64
}

// NewLedger 建台账。ttl 小于等于零时取 30 分钟，过期信号由清理 worker 摘除。

  func NewLedger(ttl time.Duration) *Ledger {
	  if ttl <= 0 {
		   ttl = 30 * time.Minute
	  }
	  return &Ledger{
		   rows: make([]AbuseSignal, 0, 64), cap: defaultLedgerCap, ttl: ttl,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟，供回归夹具推进窗口。

  func (l *Ledger) WithClock(c func() time.Time) *Ledger {
	   if l != nil && c != nil {
		  l.mu.Lock()
		  l.clock = c
		   l.mu.Unlock()
	   }
	   return l
   }

   // SetTTL 调整信号保留时长。非正值保持原值。
   func (l *Ledger) SetTTL(ttl time.Duration) {
	  if l == nil || ttl <= 0 {
		  return
	  }
	   l.mu.Lock()
	  l.ttl = ttl
	   l.mu.Unlock()
   }

// SetCap 调整环形表上限，最少 64 条。

  func (l *Ledger) SetCap(n int) {
	  if l == nil {
		  return
	   }
	  if n < 64 {
		  n = 64
	  }
	  l.mu.Lock()
	   l.cap = n
	  l.mu.Unlock()
  }

   func (l *Ledger) now() time.Time {
	   if l == nil || l.clock == nil {
		  return time.Now().UTC()
	   }
	   return l.clock()
   }

// Normalize 补齐信号字段并校验。权重为负、种类未登记、目标全空一律失败。
// 功能：统一大小写与时间，使同一用户不同入口的信号能落到同一作用域键。
// 重点：SourceKey 原样保留（可能是 dep:/wd: 键），不做改写。
// 风险点：用户为空的信号只进资产/链/全局键，个体判定会失真。
   func (l *Ledger) Normalize(sig AbuseSignal) (AbuseSignal, error) {
	   if !sig.Kind.Valid() {
		  return AbuseSignal{}, ErrSignalKind
	  }
	  if sig.Weight < 0 {
		  return AbuseSignal{}, ErrSignal
	   }
	   if sig.Weight == 0 {
		   sig.Weight = sig.Kind.BaseWeight()
	  }
	   if sig.Path == "" {
		  sig.Path = sig.Kind.HintPath()
	   }
	  if sig.Path != "" && !sig.Path.Valid() {
		  return AbuseSignal{}, ErrPath
	   }
	   sig.UserID = strings.TrimSpace(sig.UserID)
	   sig.Chain = strings.ToUpper(strings.TrimSpace(sig.Chain))
	  sig.Asset = strings.ToUpper(strings.TrimSpace(sig.Asset))
	  sig.SourceKey = strings.TrimSpace(sig.SourceKey)
	  sig.Note = strings.TrimSpace(sig.Note)
	  if sig.UserID == "" && sig.Chain == "" && sig.Asset == "" {
		  return AbuseSignal{}, ErrSignal
	  }
	  if strings.Contains(sig.UserID, "|") {
		  return AbuseSignal{}, ErrSignal
	  }
	   if sig.ID == "" {
		   sig.ID = uuid.NewString()
	   }
	   if sig.At.IsZero() {
		  sig.At = l.now()
	   } else {
		   sig.At = sig.At.UTC()
	  }
	  return sig, nil
   }

// Ingest 记一条信号。返回补齐后的信号，便于调用方落库与审计。

   func (l *Ledger) Ingest(sig AbuseSignal) (AbuseSignal, error) {
	  if l == nil {
		  return AbuseSignal{}, ErrStore
	  }
	   row, err := l.Normalize(sig)
	  if err != nil {
		   return AbuseSignal{}, err
	   }
	   l.mu.Lock()
	   defer l.mu.Unlock()
	   l.rows = append(l.rows, row)
	   l.ingest++
	  if len(l.rows) > l.cap {
		   drop := len(l.rows) - l.cap
		  l.rows = append([]AbuseSignal{},
			   l.rows[drop:]...)
		  l.dropped += int64(drop)
	  }
	   return row, nil
  }

// Window 取某作用域键在窗口内的信号。窗口非正时按 ttl 处理。
  func (l *Ledger) Window(key string, win time.Duration) []AbuseSignal {
	  if l == nil {
		   return nil
	  }
	  scope, user, chain, asset, err := ParseKey(key)
	   if err != nil {
		  return nil
	  }
	  if win <= 0 {
		  win = l.ttl
	  }
	   cut := l.now().Add(-win)
	   l.mu.Lock()
	   defer l.mu.Unlock()
	   out := make([]AbuseSignal, 0, 16)
	  for _, r := range l.rows {
		   if r.At.Before(cut) {
			   continue
		   }
		   if !matchScope(r, scope, user, chain, asset) {
			   continue
		  }
		   out = append(out, r)
	  }
	   return out
  }

  func matchScope(sig AbuseSignal, scope Scope, user, chain, asset string) bool {
	  switch scope {
	  case ScopeGlobal:
		  return true
	  case ScopeUser:
		  return sig.UserID != "" && sig.UserID == user
	  case ScopeAsset:
		   return sig.Asset != "" && sig.Asset == asset
	   case ScopeChain:
		   return sig.Chain != "" && sig.Chain == chain
	  }
	   return false
  }

// ActiveKeys 窗口内出现过信号的全部作用域键（含 global）。聚合 worker 按此收敛遍历范围。

   func (l *Ledger) ActiveKeys(win time.Duration) []string {
	   if l == nil {
		   return nil
	   }
	   if win <= 0 {
		  win = l.ttl
	  }
	  cut := l.now().Add(-win)
	   l.mu.Lock()
	   rows := make([]AbuseSignal, 0, len(l.rows))
	   for _, r := range l.rows {
		  if r.At.Before(cut) {
			   continue
		  }
		  rows = append(rows, r)
	  }
	   l.mu.Unlock()
	  seen := map[string]struct{}{}
	  out := make([]string, 0, 8)
	   for _, r := range rows {
		   for _, k := range KeysFor(r) {
			   if _, ok := seen[k]; ok {
				   continue
			  }
			  seen[k] = struct{}{}
			   out = append(out, k)
		  }
	   }
	   return out
   }

// Recent 最近若干条信号，供运营读面。

  func (l *Ledger) Recent(limit int) []AbuseSignal {
	  if l == nil {
		   return nil
	   }
	  if limit <= 0 || limit > 500 {
		   limit = 50
	   }
	  l.mu.Lock()
	  defer l.mu.Unlock()
	   if len(l.rows) <= limit {
		   return append([]AbuseSignal{}, l.rows...)
	   }
	   return append([]AbuseSignal{}, l.rows[len(l.rows)-limit:]...)
   }

// Prune 摘除过期信号，返回摘除条数。清理 worker 周期调用。

  func (l *Ledger) Prune() int {
	   if l == nil {
		  return 0
	  }
	   cut := l.now().Add(-l.ttl)
	   l.mu.Lock()
	  defer l.mu.Unlock()
	   keep := make([]AbuseSignal,
		   0, len(l.rows))
	   for _, r := range l.rows {
		  if r.At.Before(cut) {
			   continue
		   }
		   keep = append(keep, r)
	  }
	   n := len(l.rows) - len(keep)
	   l.rows = keep
	   return n
   }

// Len 当前台账条数。
  func (l *Ledger) Len() int {
	   if l == nil {
		  return 0
	   }
	  l.mu.Lock()
	   defer l.mu.Unlock()
	   return len(l.rows)
   }

// LedgerStats 台账读面。

type LedgerStats struct {
	Rows     int   `json:"rows"`
	Cap      int   `json:"cap"`
	TTLSec   int   `json:"ttl_sec"`
	Ingested int64 `json:"ingested"`
	Dropped  int64 `json:"dropped"`
}

// Stats 台账统计。

  func (l *Ledger) Stats() LedgerStats {
	  if l == nil {
		   return LedgerStats{}
	  }
	  l.mu.Lock()
	   defer l.mu.Unlock()
	  return LedgerStats{
		   Rows: len(l.rows), Cap: l.cap, TTLSec: int(l.ttl / time.Second),
		   Ingested: l.ingest,
		   Dropped:  l.dropped,
	   }
   }
