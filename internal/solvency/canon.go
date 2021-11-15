package solvency

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func utc(t time.Time) time.Time {
	   if t.IsZero() {
		   return time.Time{}
	  }
	  return t.UTC().Truncate(time.Millisecond)
   }

  func or0(s string) string {
	  s = strings.TrimSpace(s)
	   if s == "" {
		   return "0"
	   }
	   return s
  }

   func normAmt(s string) (string, error) {
	   a, err := money.ParseBook(or0(s))
	   if err != nil {
		   return "", ErrAmount
	   }
	   return a.String(), nil
  }

  func addAmt(parts ...string) (string, error) {
	   sum := money.Zero()
	  for _, p := range parts {
		   a, err := money.ParseBook(or0(p))
		   if err != nil {
			  return "", ErrAmount
		  }
		  next, err := sum.Add(a)
		  if err != nil {
			   return "", err
		  }
		  sum = next
	   }
	   return sum.String(), nil
   }

  func cmpAmt(a, b string) (int, error) {
	  l, err := money.ParseBook(or0(a))
	   if err != nil {
		   return 0, ErrAmount
	   }
	  r, err := money.ParseBook(or0(b))
	   if err != nil {
		  return 0, ErrAmount
	  }
	   return l.Cmp(r), nil
   }

  func subAmt(a, b string) (string, error) {
	   l, err := money.ParseBook(or0(a))
	   if err != nil {
		   return "", ErrAmount
	   }
	  r, err := money.ParseBook(or0(b))
	   if err != nil {
		  return "", ErrAmount
	   }
	  out, err := l.Sub(r)
	   if err != nil {
		  return "", ErrAmount
	  }
	  return out.String(), nil
   }

// NormBook 把展示名 funding 收成账本科目 spot，其余科目保持账本类型名。

  func NormBook(s string) (string, error) {
	  s = strings.ToLower(strings.TrimSpace(s))
	  switch s {
	   case "funding", "spot":
		  return "spot", nil
	   case "wallet", "trading", "margin", "isolated", "futures", "earn":
		  return s, nil
	  default:
		   return "", ErrLeaf
	  }
  }

   func (l LiabilityLeaf) Key() string {
	  return l.UserID + "|" + l.Book + "|" + l.Asset
   }

// Canonical 绑定身份、金额与采集时的围栏。前缀避免与工程默克尔叶串互通。

  func (l LiabilityLeaf) Canonical() string {
	   return strings.Join([]string{
		   leafPrefix,
		  strings.TrimSpace(l.UserID),
		   strings.TrimSpace(l.Book),
		  strings.ToUpper(strings.TrimSpace(l.Asset)),
		   strings.TrimSpace(l.Available),
		   strings.TrimSpace(l.Frozen),
		   strings.TrimSpace(l.Total),
		  strconv.FormatInt(l.FenceEpoch,
			   10),
		   strings.TrimSpace(l.FenceToken),
	   }, "|")
  }

  func (l LiabilityLeaf) Hash() []byte {
	  sum := sha256.Sum256([]byte(l.Canonical()))
	  return sum[:]
   }

   func (l LiabilityLeaf) Hex() string { return hex.EncodeToString(l.Hash()) }

// BookDigest 对已排序叶的规范串做摘要。封存时用同一函数重算，不一致则拒绝封存。
  func BookDigest(leaves []LiabilityLeaf) string {
	   cp := append([]LiabilityLeaf(nil),
		  leaves...)
	   sortLeaves(cp)
	   h := sha256.New()
	  for _, l := range cp {
		   _, _ = h.Write([]byte(l.Canonical()))
		   _, _ = h.Write([]byte{'\n'})
	   }
	   return hex.EncodeToString(h.Sum(nil))
  }

// SealDigest 把根、叶数、围栏、账面摘要、覆盖结论绑在一起。
// 发布只接受与此摘要一致的封存结果，草稿根没有摘要。
   func SealDigest(a Attestation) string {
	   s := strings.Join([]string{
		  sealPrefix,
		  a.ID,
		  a.Root,
		  strconv.Itoa(a.LeafCount),
		   strconv.FormatInt(a.FenceEpoch, 10),
		   a.FenceToken,
		   a.BookDigest,
		   a.Coverage,
		   a.Scope,
		   a.Class,
	   }, "|")
	  sum := sha256.Sum256([]byte(s))
	  return hex.EncodeToString(sum[:])
   }

   func SignCanon(a Attestation, seal string) string {
	   return strings.Join([]string{
		  sigPrefix,
		   a.ID,
		  a.Root,
		   strconv.Itoa(a.LeafCount),
		   strconv.FormatInt(a.FenceEpoch, 10),
		   a.WindowStart.UTC().Format(time.RFC3339Nano),
		   a.WindowEnd.UTC().Format(time.RFC3339Nano),
		  a.Class,
		   a.Coverage,
		  a.Scope,
		  seal,
	  }, "|")
  }

  func classOf(a Attestation) (string, string) {
	  if a.Coverage == CoverageCovered && strings.TrimSpace(a.Scope) == "" {
		  return ClassPublishable, DisclaimerPublishable
	   }
	  return ClassInternal, DisclaimerInternal
   }

  func scopeAllows(scope, asset string) bool {
	   scope = strings.TrimSpace(scope)
	   if scope == "" {
		  return true
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   for _, p := range strings.Split(scope, ",") {
		  if strings.ToUpper(strings.TrimSpace(p)) == asset {
			   return true
		   }
	  }
	   return false
   }

  func joinScope(assets []string) string {
	   out := make([]string, 0, len(assets))
	   seen := map[string]struct{}{}
	   for _, a := range assets {
		   a = strings.ToUpper(strings.TrimSpace(a))
		  if a == "" {
			   continue
		  }
		  if _, ok := seen[a]; ok {
			  continue
		  }
		   seen[a] = struct{}{}
		   out = append(out, a)
	  }
	  return strings.Join(out, ",")
  }
