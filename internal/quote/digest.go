package quote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Digest 规范化摘要。字段顺序固定，便于 golden 。
type Digest struct {
	Pair string `json:"pair"`
	Rate string `json:"rate"`
	FeeBPS int `json:"fee_bps"`
	FeeAmt string `json:"fee_amt"`
	AsOf   string `json:"as_of"`
	TTLNS  int64  `json:"ttl_ns"`
	Source string `json:"source"`
	Class  string `json:"class,omitempty"`
	Levels int `json:"levels"`
}

   func (q Quote) Digest(now time.Time) Digest {
	  cls := ""
	  if !now.IsZero() {
		   cls = Classify(q, now).String()
	   }
	   n := 0
	  for _, lv := range q.Levels {
		  if lv.Px.IsPositive() && lv.Sz.IsPositive() {
			   n++
		  }
	   }
	   asof := ""
	   if !q.AsOf.IsZero() {
		   asof = q.AsOf.UTC().Format(time.RFC3339Nano)
	  }
	   return Digest{
		   Pair: q.Pair,
		  Rate: q.Rate.String(),
		  FeeBPS: q.FeeBPS,
		   FeeAmt: q.FeeAmount.String(),
		  AsOf:   asof,
		  TTLNS:  int64(q.TTL),
		  Source: NormalizeSource(q.Source),
		   Class:  cls,
		  Levels: n,
	  }
  }

  func (d Digest) Hash() string {
	  raw, err := json.Marshal(d)
	   if err != nil {
		  return ""
	  }
	   sum := sha256.Sum256(raw)
	  return hex.EncodeToString(sum[:])
   }

// BoundaryCase TTL 用例描述。

type BoundaryCase struct {
	Name string        `json:"name"`
	Age  time.Duration `json:"age"`
	TTL  time.Duration `json:"ttl"`
	Want string        `json:"want"` // ok / stale / empty
	Comment string `json:"comment"`
}

// DefaultBoundaryTable 锁死 age 与 TTL 关系。

  func DefaultBoundaryTable() []BoundaryCase {
	   ttl := DefaultSwapTTL
	   return []BoundaryCase{
		   {Name: "age_zero", Age: 0, TTL: ttl, Want: "ok", Comment: "刚写入"},
		  {Name: "age_half", Age: ttl / 2, TTL: ttl, Want: "ok", Comment: "窗口中位"},
		  {Name: "age_eq_ttl", Age: ttl, TTL: ttl, Want: "ok", Comment: "恰等于 TTL 仍有效"},
		  {Name: "age_ttl_plus_ns", Age: ttl + time.Nanosecond, TTL: ttl, Want: "stale", Comment: "越过 1ns 即过期"},
		  {Name: "age_ttl_plus_1s", Age: ttl + time.Second, TTL: ttl, Want: "stale", Comment: "明显过期"},
		   {Name: "ttl_zero", Age: 0, TTL: 0, Want: "empty", Comment: "未初始化窗口"},
		   {Name: "ttl_neg", Age: 0, TTL: -time.Second, Want: "empty", Comment: "负窗口视为未初始化"},
		  {Name: "asof_zero_implied", Age: -1, TTL: ttl, Want: "empty", Comment: "AsOf 为零"},
	  }
  }

// PairIsolationKeys 多币对夹具键。

   func PairIsolationKeys() []string {
	   return []string{"BTC/USDT", "ETH/USDT", "SOL/USDT", "TRX/USDT"}
  }

   func FormatDurationNS(d time.Duration) string {
	   return strconv.FormatInt(int64(d), 10)
   }

   func JoinKeys(keys []string) string {
	  cp := append([]string(nil), keys...)
	  sort.Strings(cp)
	   return strings.Join(cp, ",")
  }
