package balproofgate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// BuildSnapshot 由活账观察构造快照根。金额必须可按账本精度解析。

  func BuildSnapshot(view LiveView) (LiveSnapshotRoot, error) {
	  avail := strings.TrimSpace(view.Available)
	   frozen := strings.TrimSpace(view.Frozen)
	   if avail == "" {
		  avail = "0"
	  }
	   if frozen == "" {
		   frozen = "0"
	   }
	   tot, err := totalOf(avail, frozen)
	   if err != nil {
		  return LiveSnapshotRoot{}, err
	  }
	   obs := view.ObserveGen
	  if obs == 0 {
		   obs = FingerprintObserveGen(avail, frozen)
	  }
	   at := view.At.UTC()
	  if at.IsZero() {
		  at = time.Now().UTC()
	  }
	  root := LiveSnapshotRoot{
		  Available: avail,
		  Frozen:    frozen,
		   Total:     tot,
		  ObserveGen:    obs,
		   ObservedAt:    at,
		  IsolationMark: IsolationMark,
	   }
	  root.RootDigest = SnapshotDigest(root)
	  return root, nil
   }

// SnapshotDigest 计算快照根摘要。禁止混入 por/stmt 字段。

   func SnapshotDigest(r LiveSnapshotRoot) string {
	  can := strings.Join([]string{
		  strings.TrimSpace(r.Available),
		   strings.TrimSpace(r.Frozen),
		  strings.TrimSpace(r.Total),
		  itoa64(r.ObserveGen),
		  r.ObservedAt.UTC().Format(time.RFC3339Nano),
		  IsolationMark,
	  }, "|")
	   sum := sha256.Sum256([]byte(can))
	  return hex.EncodeToString(sum[:])
  }

// LeaseCanonical 租约签名规范串。
   func LeaseCanonical(userID, book, asset, nonce string, snap LiveSnapshotRoot, issuedAt time.Time, gen int64) string {
	  return strings.Join([]string{
		  strings.TrimSpace(userID),
		  strings.TrimSpace(book),
		  strings.ToUpper(strings.TrimSpace(asset)),
		   strings.TrimSpace(snap.Available),
		   strings.TrimSpace(snap.Frozen),
		  itoa64(snap.ObserveGen),
		  itoa64(gen),
		  issuedAt.UTC().Format(time.RFC3339Nano),
		  strings.TrimSpace(nonce),
		   IsolationMark,
		  strings.TrimSpace(snap.RootDigest),
	  }, "|")
  }

// SignHMAC 对规范串做 HMAC-SHA256。
   func SignHMAC(secret, canonical string) string {
	  mac := hmac.New(sha256.New, []byte(secret))
	   _, _ = mac.Write([]byte(canonical))
	  return hex.EncodeToString(mac.Sum(nil))
   }

// VerifyHMAC 恒定时间比较。
  func VerifyHMAC(secret, canonical, sig string) error {
	   want := SignHMAC(secret, canonical)
	  if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(sig))) {
		   return ErrSig
	  }
	  return nil
   }

   func totalOf(avail, frozen string) (string, error) {
	   a, err := money.ParseBook(avail)
	  if err != nil {
		   return "", ErrAmountInvalid
	   }
	  f, err := money.ParseBook(frozen)
	   if err != nil {
		  return "", ErrAmountInvalid
	  }
	  sum, err := a.Add(f)
	  if err != nil {
		  return "", ErrAmountInvalid
	  }
	  return sum.String(), nil
   }

// ClassifyDrift 比较签发快照与当前活账，返回漂移类型；无漂移返回空。

  func ClassifyDrift(snap LiveSnapshotRoot, live LiveView) DriftKind {
	  avail := strings.TrimSpace(live.Available)
	   frozen := strings.TrimSpace(live.Frozen)
	   if avail == "" {
		   avail = "0"
	  }
	   if frozen == "" {
		  frozen = "0"
	   }
	   obs := live.ObserveGen
	   if obs == 0 {
		   obs = FingerprintObserveGen(avail, frozen)
	  }
	   if avail != strings.TrimSpace(snap.Available) {
		   if cmpAmount(avail, snap.Available) > 0 {
			   return DriftAvailCredit
		  }
		   return DriftAvailDebit
	  }
	   if frozen != strings.TrimSpace(snap.Frozen) {
		   if cmpAmount(frozen, snap.Frozen) > 0 {
			  return DriftFrozenUp
		  }
		   return DriftFrozenDown
	   }
	   if obs != snap.ObserveGen {
		  return DriftObserveGen
	   }
	   return ""
  }

   func cmpAmount(a, b string) int {
	  aa, err1 := money.ParseBook(a)
	   bb, err2 := money.ParseBook(b)
	   if err1 != nil || err2 != nil {
		   if a > b {
			   return 1
		  }
		  if a < b {
			   return -1
		   }
		   return 0
	   }
	  return aa.Cmp(bb)
   }

// DriftDigest 漂移证明摘要。

  func DriftDigest(p DriftProof) string {
	  can := strings.Join([]string{
		  strings.TrimSpace(p.LeaseID),
		  strings.TrimSpace(p.UserID),
		  strings.TrimSpace(p.Book),
		   strings.ToUpper(strings.TrimSpace(p.Asset)),
		  string(p.Kind),
		   strings.TrimSpace(p.IssuedAvail),
		   strings.TrimSpace(p.IssuedFrozen),
		   itoa64(p.IssuedObsGen),
		  strings.TrimSpace(p.LiveAvail),
		   strings.TrimSpace(p.LiveFrozen),
		   itoa64(p.LiveObsGen),
		   IsolationMark,
	  }, "|")
	  sum := sha256.Sum256([]byte(can))
	  return hex.EncodeToString(sum[:])
  }

// RejectPorOrStmtField 防守：导出或落证时若调用方试图塞入 por/stmt 根则失败关闭。
   func RejectPorOrStmtField(label string) error {
	   l := strings.ToLower(strings.TrimSpace(label))
	   if strings.Contains(l, "por_root") || strings.Contains(l, "stmt_manifest") || strings.Contains(l, "leddiff") {
		   return ErrIsolationViolation
	   }
	   return nil
  }
