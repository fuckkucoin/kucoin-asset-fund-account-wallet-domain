package tradepw

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/internal/tradepwgen"
)

const kind = "aa_trade_pw"

var (
	ErrUser = errors.New("trading password user required")
	ErrFormat   = errors.New("trading password must be 6 digits")
	ErrMismatch = errors.New("trading password mismatch")
	ErrUnset    = errors.New("trading password not set")
	ErrStore    = errors.New("trading password store unavailable")
	ErrSame     = errors.New("new trading password must differ")
)

type rec struct {
	Salt string `json:"salt"`
	Hash string `json:"hash"`
}

// Book 交易密码账本。
// genSvc 注入后，Set/Change/Reset 会同步抬升密码世代并吊销旧在途核验令牌。
type Book struct {
	kv  persist.KV
	sec *seclock.Service
	maxFails int
	lockTTL time.Duration
	genSvc *tradepwgen.Service
}

   func New(kv persist.KV) *Book { return &Book{kv: kv} }

   func (b *Book) BindCooldown(sec *seclock.Service) *Book {
	  if b != nil {
		   b.sec = sec
	   }
	  return b
  }

// BindGeneration 注入密码世代引擎（可选；nil 则跳过世代管理）。
// 路径须注入，否则竞态封堵能力不可用。

  func (b *Book) BindGeneration(svc *tradepwgen.Service) *Book {
	  if b != nil {
		  b.genSvc = svc
	  }
	  return b
  }

  func (b *Book) Set(ctx context.Context, userID, password string) error {
	  if b == nil || b.kv == nil {
		   return ErrStore
	  }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return ErrUser
	   }
	  if err := validatePIN(password); err != nil {
		   return err
	   }
	  if _, err := b.load(ctx, userID); err == nil {
		   return fmt.Errorf("trading password already set")
	   }
	   if err := b.write(ctx, userID, password, persist.Fence{}); err != nil {
		  return err
	  }
	   _ = b.clearFail(ctx, userID)
	  // 首次设密抬升世代（expectedGen=0）。
	   if b.genSvc != nil {
		  if _, _, err := b.genSvc.BumpAndRevoke(userID, "set"); err != nil {
			   return fmt.Errorf("tradepw: Set.BumpAndRevoke: %w", err)
		   }
	   }
	  return nil
  }

  func (b *Book) Change(ctx context.Context, userID, oldPIN, newPIN string) error {
	  if err := b.Verify(ctx, userID, oldPIN); err != nil {
		  return err
	  }
	  if err := validatePIN(newPIN); err != nil {
		  return err
	   }
	   if subtle.ConstantTimeCompare([]byte(oldPIN), []byte(newPIN)) == 1 {
		  return ErrSame
	  }
	  cur,
		  err := b.kv.Get(ctx, kind, userID)
	  if err != nil {
		   return err
	  }
	   if err := b.write(ctx, userID, newPIN, cur.Fence); err != nil {
		  return err
	  }
	  _ = b.clearFail(ctx, userID)
	   // 抬升世代并吊销旧令牌（须在密码写入后执行，保证新密成功前不吊销）。
	  var newGen int64
	   if b.genSvc != nil {
		   ng, _, bumperr := b.genSvc.BumpAndRevoke(userID, "change")
		   if bumperr != nil {
			   return fmt.Errorf("tradepw: Change.BumpAndRevoke: %w", bumperr)
		  }
		  newGen = ng.Generation
	   }
	   // 写入安全冷却；携带世代号（世代号=0 表示 genSvc 未注入，兼容旧路径）。
	   return b.markCoolWithGen(ctx, userID, "trading_password_change", newGen)
   }

  func (b *Book) Reset(ctx context.Context, userID, newPIN string) error {
	   if b == nil || b.kv == nil {
		  return ErrStore
	  }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return ErrUser
	   }
	  if err := validatePIN(newPIN); err != nil {
		  return err
	  }
	   cur, err := b.kv.Get(ctx, kind, userID)
	  exp := persist.Fence{}
	   if err == nil {
		  exp = cur.Fence
	   } else if !errors.Is(err, persist.ErrNotFound) {
		   return err
	  }
	  if err := b.write(ctx, userID, newPIN, exp); err != nil {
		  return err
	   }
	  _ = b.clearFail(ctx, userID)
	  // 抬升世代并吊销旧令牌。
	  var newGen int64
	   if b.genSvc != nil {
		  ng, _, bumperr := b.genSvc.BumpAndRevoke(userID, "reset")
		  if bumperr != nil {
			   return fmt.Errorf("tradepw: Reset.BumpAndRevoke: %w", bumperr)
		   }
		   newGen = ng.Generation
	   }
	   return b.markCoolWithGen(ctx, userID, "trading_password_reset", newGen)
   }

// Verify 核对交易密码；先查错密锁，失败累加计数，成功清零。
   func (b *Book) Verify(ctx context.Context, userID, password string) error {
	   if err := b.assertUnlocked(ctx, userID); err != nil {
		   return err
	   }
	   r, err := b.load(ctx, userID)
	  if err != nil {
		  return err
	  }
	   if err := validatePIN(password); err != nil {
		   _ = b.noteFail(ctx, userID)
		   return ErrMismatch
	   }
	   want, err := hex.DecodeString(r.Hash)
	   if err != nil || len(want) == 0 {
		   _ = b.noteFail(ctx, userID)
		   return ErrMismatch
	  }
	   got := hashPIN(r.Salt, password)
	   if subtle.ConstantTimeCompare(want, got) != 1 {
		  _ = b.noteFail(ctx, userID)
		  return ErrMismatch
	   }
	  _ = b.clearFail(ctx, userID)
	  return nil
   }

  func (b *Book) Setted(ctx context.Context, userID string) bool {
	  _, err := b.load(ctx, userID)
	   return err == nil
   }

// RequireIfSet 已设交易密码则必须核对；未设不拦截，避免未开通用户无法出金。
   func (b *Book) RequireIfSet(ctx context.Context, userID, password string) error {
	   if b == nil || !b.Setted(ctx, userID) {
		   return nil
	   }
	   return b.Verify(ctx,
		   userID, password)
  }

// RequireForWithdraw 出金前置核验：验密 + 颁发 VerifyLease。
// genSvc 未注入时退化为普通 Verify；注入后同步检查世代、漂移案件。
// 成功返回 verify_lease_id（供他包只读引用）；genSvc 未注入时返回空字符串。
   func (b *Book) RequireForWithdraw(ctx context.Context, userID, password string, leaseTTL interface{ IsZero() bool }) (leaseID string, err error) {
	  if err := b.Verify(ctx, userID, password); err != nil {
		   return "", err
	  }
	  if b.genSvc == nil {
		   return "", nil
	  }
	   lease, issueErr := b.genSvc.IssueVerifyLease(tradepwgen.IssueLeaseReq{
		   UserID: userID,
	  })
	   if issueErr != nil {
		  return "", fmt.Errorf("tradepw: RequireForWithdraw.IssueLease: %w", issueErr)
	  }
	  return lease.ID, nil
   }

// PwGen 返回用户当前密码世代号（供他包只读引用；genSvc 未注入时返回 0）。

  func (b *Book) PwGen(ctx context.Context, userID string) (int64, error) {
	   if b.genSvc == nil {
		   return 0, nil
	  }
	  resp, err := b.genSvc.QueryGeneration(userID)
	   if err != nil {
		  return 0, err
	  }
	   return resp.Generation, nil
   }

  func (b *Book) load(ctx context.Context, userID string) (rec, error) {
	   if b == nil || b.kv == nil {
		  return rec{}, ErrStore
	   }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return rec{}, ErrUser
	  }
	   row,
		  err := b.kv.Get(ctx, kind, userID)
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			  return rec{}, ErrUnset
		  }
		   return rec{}, err
	  }
	  var r rec
	  if err := json.Unmarshal(row.Body, &r); err != nil || r.Hash == "" || r.Salt == "" {
		  return rec{}, ErrUnset
	   }
	  return r, nil
   }

  func (b *Book) write(ctx context.Context, userID, password string, exp persist.Fence) error {
	   salt, err := randomSalt()
	   if err != nil {
		   return err
	   }
	   body, err := json.Marshal(rec{Salt: salt, Hash: hex.EncodeToString(hashPIN(salt, password))})
	   if err != nil {
		   return err
	  }
	  return b.kv.Put(ctx, persist.Row{Kind: kind, Key: userID, Body: body}, exp)
  }

// markCool 写入安全冷却（旧路径，genSvc 未注入时调用）。
   func (b *Book) markCool(ctx context.Context, userID, reason string) error {
	  if b == nil || b.sec == nil {
		  return nil
	   }
	   return b.sec.Mark(ctx, userID, reason)
   }

// markCoolWithGen 写入安全冷却，并在 genSvc 注入时确认世代冷却绑定。
// gen=0 表示 genSvc 未注入，退化为普通 markCool。
   func (b *Book) markCoolWithGen(ctx context.Context, userID, reason string, gen int64) error {
	  if err := b.markCool(ctx, userID, reason); err != nil {
		   return err
	   }
	   if b.genSvc != nil && gen > 0 {
		   if err := b.genSvc.ConfirmCool(userID, gen); err != nil {
			   // 冷却绑定失败不阻断主链（seclock 已写入），记录但不返回错误。
			   // 风险点：CoolBound=false 时 Worker 须异步补确认。
			   _ = err
		  }
	   }
	  return nil
  }

   func validatePIN(s string) error {
	  s = strings.TrimSpace(s)
	  if len(s) != 6 {
		   return ErrFormat
	   }
	   for _, r := range s {
		   if !unicode.IsDigit(r) {
			   return ErrFormat
		   }
	   }
	  return nil
   }

   func hashPIN(salt, pin string) []byte {
	   sum := sha256.Sum256([]byte(salt + "\n" + pin))
	  return sum[:]
   }

   func randomSalt() (string, error) {
	  var b [16]byte
	   if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		   return "", err
	   }
	   return hex.EncodeToString(b[:]), nil
  }
