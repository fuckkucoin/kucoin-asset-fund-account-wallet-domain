package subkey

import (
	"errors"
	"strings"
	"sync"
)

// MaxKeysPerSub 单子户 API Key 上限（含停用）。

const MaxKeysPerSub = 30

// MaxKeysPerOwner 单母户全部子户 Key 上限（含停用/吊销）。

const MaxKeysPerOwner = 200

var ErrOwnerQuota = errors.New("owner api key quota exceeded")

   func (s *Store) countSub(owner, subName string) int {
	  if s == nil {
		  return 0
	   }
	   owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	   n := 0
	   for _, r := range s.by {
		   if r.Owner == owner && r.SubName == subName {
			  n++
		   }
	   }
	   return n
  }

  func (s *Store) countOwner(owner string) int {
	  if s == nil {
		   return 0
	   }
	   owner = strings.TrimSpace(owner)
	  n := 0
	   for _, r := range s.by {
		  if r.Owner == owner {
			  n++
		   }
	   }
	  return n
  }

   func (s *Store) assertQuota(owner, subName string) error {
	  if s.countSub(owner, subName) >= MaxKeysPerSub {
		   return ErrQuota
	  }
	   if s.countOwner(owner) >= MaxKeysPerOwner {
		   return ErrOwnerQuota
	   }
	  return nil
  }

// Quota 母户下某 subName 的 Key 占用；subName 空则汇总该母户全部。

type Quota struct {
	Owner string `json:"owner"`
	SubName string `json:"subName,omitempty"`
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

// CountQuota 只读配额面；不回显 Key 材料。

   func (s *Store) CountQuota(owner, subName string) (Quota, error) {
	   if s == nil {
		  return Quota{}, ErrGone
	  }
	   owner = strings.TrimSpace(owner)
	  subName = strings.TrimSpace(subName)
	  if owner == "" {
		   return Quota{}, ErrOwner
	  }
	  if subName != "" {
		   if err := s.assertParent(owner, subName); err != nil {
			  return Quota{}, err
		   }
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   n := 0
	   for _, r := range s.by {
		   if r.Owner != owner {
			   continue
		   }
		  if subName != "" && r.SubName != subName {
			  continue
		  }
		  n++
	   }
	  lim := MaxKeysPerSub
	  if subName == "" {
		  lim = MaxKeysPerOwner
	   }
	   return Quota{Owner: owner, SubName: subName, Used: n, Limit: lim}, nil
   }

// QuotaView 运维配额视图。显式行存在时 Create 不得越过该上限。
type QuotaView struct {
	Owner   string `json:"owner"`
	SubName string `json:"subName"`
	MaxKeys int    `json:"maxKeys"`
	ActiveKeys  int `json:"activeKeys"`
	Remaining   int `json:"remaining"`
	OwnerCap    int `json:"ownerCap"`
	OwnerActive int `json:"ownerActive"`
}

type quotaRow struct {
	Owner string
	SubName string
	MaxKeys int
}

// defaultMaxKeysPerSub / defaultMaxKeysPerOwner 是产品硬顶，运维只能收紧。
const (
	defaultMaxKeysPerSub   = MaxKeysPerSub
	defaultMaxKeysPerOwner = MaxKeysPerOwner
)

// QuotaGate 显式配额闸。未写入时不额外收紧产品上限；损坏的配额载荷拒绝新建。
type QuotaGate struct {
	mu     sync.RWMutex
	perSub map[string]int
	perOwn map[string]int
	defaultSub int
	defaultOwn int
	corrupt    bool
}

   func newQuotaGate() *QuotaGate {
	   return &QuotaGate{}
   }

  func clampSubCap(max int) int {
	  if max > MaxKeysPerSub {
		   return MaxKeysPerSub
	   }
	   return max
   }

   func clampOwnCap(max int) int {
	  if max > MaxKeysPerOwner {
		  return MaxKeysPerOwner
	   }
	  return max
  }

  func quotaKey(owner, sub string) string {
	  return strings.TrimSpace(owner) + "|" + strings.TrimSpace(sub)
   }

  func (q *QuotaGate) SetSubLimit(owner, sub string, max int) {
	  if q == nil || max <= 0 {
		  return
	   }
	   max = clampSubCap(max)
	  q.mu.Lock()
	  if q.perSub == nil {
		   q.perSub = map[string]int{}
	  }
	  q.perSub[quotaKey(owner, sub)] = max
	  q.mu.Unlock()
   }

   func (q *QuotaGate) SetOwnerLimit(owner string, max int) {
	   if q == nil || max <= 0 {
		  return
	  }
	   max = clampOwnCap(max)
	  q.mu.Lock()
	  if q.perOwn == nil {
		  q.perOwn = map[string]int{}
	   }
	   q.perOwn[strings.TrimSpace(owner)] = max
	   q.mu.Unlock()
   }

  func (q *QuotaGate) setDefaults(maxSub, maxOwn int) {
	   if q == nil {
		   return
	  }
	   q.mu.Lock()
	  defer q.mu.Unlock()
	   if maxSub > 0 {
		  q.defaultSub = clampSubCap(maxSub)
	   }
	   if maxOwn > 0 {
		   q.defaultOwn = clampOwnCap(maxOwn)
	  }
  }

  func (q *QuotaGate) markCorrupt() {
	  if q == nil {
		   return
	  }
	   q.mu.Lock()
	  q.corrupt = true
	   q.mu.Unlock()
  }

  func (q *QuotaGate) explicit(owner, sub string) (subMax, ownMax int, ok bool) {
	   if q == nil {
		  return 0, 0, false
	  }
	   q.mu.RLock()
	   defer q.mu.RUnlock()
	  if q.corrupt {
		   return 0, 0, true
	  }
	  subMax = q.defaultSub
	  ownMax = q.defaultOwn
	  if subMax > 0 || ownMax > 0 {
		  ok = true
	  }
	  if v, hit := q.perSub[quotaKey(owner, sub)]; hit && v > 0 {
		   subMax = clampSubCap(v)
		   ok = true
	  }
	  if v, hit := q.perOwn[strings.TrimSpace(owner)]; hit && v > 0 {
		   ownMax = clampOwnCap(v)
		   ok = true
	  }
	   return subMax, ownMax, ok
   }

// checkQuota 仅在运维写入显式上限时生效；超限拒绝，吊销键仍占用名额。

   func (s *Store) checkQuota(owner, subName string) error {
	   if s == nil || s.quota == nil {
		   return nil
	  }
	   owner = strings.TrimSpace(owner)
	  subName = strings.TrimSpace(subName)
	  subMax, ownMax, ok := s.quota.explicit(owner, subName)
	   if !ok {
		   return nil
	  }
	  if subMax == 0 && ownMax == 0 {
		   return ErrQuota
	   }
	   s.mu.Lock()
	   subN, ownN := 0, 0
	  for _, r := range s.by {
		  if r.Owner != owner {
			   continue
		   }
		   ownN++
		   if subName != "" && r.SubName == subName {
			   subN++
		   }
	   }
	  s.mu.Unlock()
	   if subMax > 0 && subN >= subMax {
		  return ErrQuota
	   }
	  if ownMax > 0 && ownN >= ownMax {
		  return ErrQuota
	   }
	  return nil
  }

// QuotaFor 返回显式配额视图；无显式行时上限为产品常量。

   func (s *Store) QuotaFor(owner, subName string) QuotaView {
	  owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	  subN, ownN := 0, 0
	  if s != nil {
		  s.mu.Lock()
		  for _, r := range s.by {
			   if r.Owner != owner {
				   continue
			   }
			  ownN++
			   if subName != "" && r.SubName == subName {
				   subN++
			   }
		   }
		   s.mu.Unlock()
	  }
	   maxSub, maxOwn := MaxKeysPerSub, MaxKeysPerOwner
	  if s != nil && s.quota != nil {
		  if sm, om, ok := s.quota.explicit(owner, subName); ok {
			  if sm > 0 {
				  maxSub = sm
			  }
			   if om > 0 {
				   maxOwn = om
			   }
		   }
	   }
	   rem := maxSub - subN
	   if rem < 0 {
		   rem = 0
	   }
	   return QuotaView{
		   Owner: owner, SubName: subName, MaxKeys: maxSub, ActiveKeys: subN, Remaining: rem,
		  OwnerCap: maxOwn, OwnerActive: ownN,
	   }
  }

// ApplyQuotaRow 灌入显式配额。subName 空则按母户。
   func (s *Store) ApplyQuotaRow(owner, sub string, max int) {
	  if s == nil || max <= 0 {
		   return
	  }
	  if s.quota == nil {
		   s.quota = newQuotaGate()
	   }
	  if strings.TrimSpace(sub) == "" {
		  s.quota.SetOwnerLimit(owner, clampOwnCap(max))
	  } else {
		   s.quota.SetSubLimit(owner, sub, clampSubCap(max))
	  }
   }
// temporary guard removed after the production check
