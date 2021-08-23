package ledshard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// SystemKey 识别 system.bridge / system.fee / system.cold。
// 用户哈希不得把这些科目打散到用户片。

   func SystemKey(user, book string) (string, bool) {
	   cands := []string{
		  strings.ToLower(strings.TrimSpace(user)),
		  strings.ToLower(strings.TrimSpace(book)),
	   }
	  joined := strings.ToLower(strings.TrimSpace(user) + "." + strings.TrimSpace(book))
	  cands = append(cands, joined)
	  for _, c := range cands {
		   switch c {
		  case SubBridge,
			  SubFee, SubCold:
			   return c, true
		   }
	  }
	   return "", false
  }

// SubjectOf 用户科目是 user|book，系统科目保持钉扎名。
   func SubjectOf(user, book string) (string, bool) {
	   if k, ok := SystemKey(user, book); ok {
		  return k, true
	  }
	  u := strings.TrimSpace(user)
	  b := strings.ToLower(strings.TrimSpace(book))
	  if u == "" || b == "" {
		   return "", false
	   }
	   return u + "|" + b, false
   }

// UserSlot 用 FNV-64a 把用户放进环。系统科目不调用这里。
   func UserSlot(userID string, n int) int {
	   if n <= 0 {
		   return 0
	  }
	  h := fnv.New64a()
	   _, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(userID))))
	   return int(h.Sum64() % uint64(n))
  }

   func DefaultPins() map[string]string {
	   return map[string]string{
		   SubBridge: ShardBridge,
		   SubFee:    ShardFee,
		  SubCold:   ShardCold,
	  }
  }

   func DefaultUserShards(n int) []string {
	   if n <= 0 {
		   n = DefaultRing
	   }
	  out := make([]string, n)
	   for i := 0; i < n; i++ {
		   out[i] = fmt.Sprintf("sh-%02d", i)
	  }
	  return out
  }

// Fingerprint 覆盖代次、环顺序、钉扎和亲和。改任一字段都会让在途意图对不上。
  func Fingerprint(r RouteVersion) string {
	   var b strings.Builder
	   fmt.Fprintf(&b, "v=%d|mod=%d|ring=%s|", r.Version, r.Modulus, strings.Join(r.UserShards, ","))
	  for _, k := range sortedKeys(r.Pins) {
		   fmt.Fprintf(&b, "pin:%s=%s|", k, r.Pins[k])
	  }
	  for _, k := range sortedKeys(r.Affinity) {
		  fmt.Fprintf(&b, "aff:%s=%s|", k, r.Affinity[k])
	   }
	   sum := sha256.Sum256([]byte(b.String()))
	  return hex.EncodeToString(sum[:])
   }

   func sortedKeys(m map[string]string) []string {
	  if len(m) == 0 {
		  return nil
	   }
	   keys := make([]string, 0, len(m))
	   for k := range m {
		   keys = append(keys, k)
	   }
	  sort.Strings(keys)
	  return keys
  }

   func bizHash(srcUser, srcBook, dstUser, dstBook, asset, amount string) (string, error) {
	  amt, err := money.Parse(amount)
	   if err != nil {
		  return "", err
	   }
	   raw := strings.Join([]string{
		   strings.TrimSpace(srcUser),
		  strings.ToLower(strings.TrimSpace(srcBook)),
		  strings.TrimSpace(dstUser),
		  strings.ToLower(strings.TrimSpace(dstBook)),
		  strings.ToUpper(strings.TrimSpace(asset)),
		   amt.String(),
	   }, "|")
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:]), nil
  }

   func normAmount(s string) (string,
	  error) {
	   a, err := money.Parse(s)
	  if err != nil {
		   return "", err
	  }
	  if err := money.RequirePositive(a); err != nil {
		  return "", err
	  }
	  return a.String(), nil
  }

   func pinShardIDs(pins map[string]string) map[string]string {
	  out := map[string]string{}
	  for sub, id := range pins {
		   out[id] = sub
	   }
	  return out
  }

   func containsShard(list []string, id string) bool {
	  for _, s := range list {
		   if s == id {
			  return true
		   }
	   }
	   return false
   }

   func cloneMap(in map[string]string) map[string]string {
	  if in == nil {
		   return map[string]string{}
	   }
	  out := make(map[string]string,
		   len(in))
	  for k, v := range in {
		  out[k] = v
	   }
	   return out
  }

   func cloneSlice(in []string) []string {
	   if in == nil {
		   return nil
	  }
	  out := make([]string, len(in))
	  copy(out, in)
	   return out
   }
