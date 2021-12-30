// 功能：验证器可插拔；口令与挑战码持久化于 KV。
// 验证器可插拔；口令与挑战码持久化于 KV。
// 环境不得在代码中固化密钥；缺校验组合须失败关闭。

// 作者：CainReed
// 日期：2021.12.29

package wdverify

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/internal/tradepw"
)

const (
	kindOTP = "aa_wd_otp"
	kindTOTP = "aa_wd_totp"
	FactorPW    = "tradepw"
	FactorTOTP  = "totp"
	FactorEmail = "email"
	FactorSMS = "sms"
	FactorAllow = "allowlist"
	otpTTL      = 5 * time.Minute
)

var (
	ErrUser   = errors.New("withdraw verify user required")
	ErrFactor = errors.New("withdraw verify factor unknown")
	ErrCode = errors.New("withdraw verify code mismatch")
	ErrCombo = errors.New("withdraw verify combo not satisfied")
	ErrStore = errors.New("withdraw verify store unavailable")
	ErrUnset   = errors.New("withdraw verify factor not bound")
	ErrExpired = errors.New("withdraw verify challenge expired")
	ErrAllow   = errors.New("withdraw destination not on allowlist")
	ErrLocked = errors.New("withdraw verify locked after failures")
)

type Challenge struct {
	ID string `json:"id"`
	UserID    string `json:"user_id"`
	Factor    string `json:"factor"`
	ExpiresAt int64  `json:"expires_at"`
}

type Presented struct {
	TradePassword string
	TOTP  string
	Email string
	SMS   string
}

// FactorError 结构化失败：标明因子与哨兵，供 HTTP 面透出。
type FactorError struct {
	Factor string
	Err error
}

   func (e *FactorError) Error() string {
	   if e == nil || e.Err == nil {
		  return ErrCombo.Error()
	   }
	   if e.Factor == "" {
		  return e.Err.Error()
	   }
	   return fmt.Sprintf("%s: %v", e.Factor, e.Err)
  }

   func (e *FactorError) Unwrap() error {
	   if e == nil {
		   return nil
	  }
	  return e.Err
   }

type Sender func(ctx context.Context, userID, factor, code string) error

// AllowChecker 地址簿白名单只读校验钩子；由上层绑定 destallow，本包不写名单。

type AllowChecker func(ctx context.Context, userID, chain, addr string) error

type Gate struct {
	kv persist.KV
	pw     *tradepw.Book
	sec    *seclock.Service
	combos [][]string
	send     Sender
	allow    AllowChecker
	maxFails int
	lockTTL  time.Duration
}

   func New(kv persist.KV, pw *tradepw.Book, combos [][]string) *Gate {
	  return &Gate{kv: kv, pw: pw, combos: normalizeCombos(combos)}
   }

   func (g *Gate) BindSender(s Sender) *Gate {
	   if g != nil {
		  g.send = s
	   }
	   return g
  }

   func (g *Gate) BindCooldown(sec *seclock.Service) *Gate {
	  if g != nil {
		  g.sec = sec
	   }
	   return g
   }

  func (g *Gate) BindAllow(fn AllowChecker) *Gate {
	  if g != nil {
		  g.allow = fn
	  }
	   return g
   }

   func (g *Gate) BindFailPolicy(maxFails int, lockFor time.Duration) *Gate {
	   if g != nil {
		   if maxFails > 0 {
			  g.maxFails = maxFails
		   }
		  if lockFor > 0 {
			  g.lockTTL = lockFor
		   }
	  }
	   return g
  }

   func (g *Gate) Combos() [][]string {
	   if g == nil {
		   return nil
	   }
	   return g.combos
   }

   func (g *Gate) SetCombos(combos [][]string) *Gate {
	   if g != nil {
		  g.combos = normalizeCombos(combos)
	  }
	  return g
   }

// Check 按配置组合核验出金因子；无组合时仅 RequireIfSet 交易密码。
   func (g *Gate) Check(ctx context.Context, userID string, p Presented) error {
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return ErrUser
	   }
	   if err := g.assertUnlocked(ctx, userID); err != nil {
		  return err
	   }
	  if len(g.combos) == 0 {
		  if g.pw == nil {
			   return nil
		  }
		  err := g.pw.RequireIfSet(ctx,
			  userID, p.TradePassword)
		  if err != nil {
			   _ = g.noteFail(ctx, userID, FactorPW)
			   return &FactorError{Factor: FactorPW, Err: err}
		   }
		  _ = g.clearFail(ctx, userID)
		  return nil
	  }
	  got := map[string]string{
		  FactorPW:    strings.TrimSpace(p.TradePassword),
		   FactorTOTP:  strings.TrimSpace(p.TOTP),
		  FactorEmail: strings.TrimSpace(p.Email),
		  FactorSMS: strings.TrimSpace(p.SMS),
	  }
	   var last error
	  for _, combo := range g.combos {
		   if err := g.match(ctx, userID, combo, got); err == nil {
			  _ = g.clearFail(ctx, userID)
			   return nil
		  } else {
			   last = err
		  }
	   }
	   _ = g.noteFail(ctx, userID, "combo")
	   if last == nil {
		   return ErrCombo
	  }
	  return last
  }

// CheckDest 白名单只读钩子；未绑定钩子时不拦截（由上层 WhitelistOnly 决定是否调用）。
   func (g *Gate) CheckDest(ctx context.Context, userID, chain, addr string) error {
	   if g == nil || g.allow == nil {
		  return nil
	  }
	  userID = strings.TrimSpace(userID)
	  chain = strings.TrimSpace(chain)
	  addr = strings.TrimSpace(addr)
	   if userID == "" || chain == "" || addr == "" {
		   return ErrAllow
	  }
	   if err := g.allow(ctx, userID, chain, addr); err != nil {
		  if errors.Is(err, ErrAllow) {
			   return err
		  }
		   return fmt.Errorf("%w: %v", ErrAllow, err)
	   }
	  return nil
  }

// CheckWithdraw 出金提交前统一组合闸：可选白名单 + 因子组合。不改状态机。
   func (g *Gate) CheckWithdraw(ctx context.Context, userID, chain, addr string, p Presented, whitelistOnly bool) error {
	  if whitelistOnly {
		  if err := g.CheckDest(ctx, userID, chain, addr); err != nil {
			  return err
		  }
	   }
	  return g.Check(ctx, userID, p)
   }

   func (g *Gate) match(ctx context.Context, userID string, combo []string, got map[string]string) error {
	  for _, f := range combo {
		  if f == FactorAllow {
			  continue // 地址白名单由 CheckDest / WhitelistOnly 路径处理
		   }
		  tok := got[f]
		   if tok == "" {
			   return &FactorError{Factor: f, Err: ErrCombo}
		  }
		   if err := g.verifyFactor(ctx, userID, f, tok); err != nil {
			   return &FactorError{Factor: f, Err: err}
		  }
	   }
	   return nil
   }

  func (g *Gate) verifyFactor(ctx context.Context, userID, factor, token string) error {
	  switch factor {
	  case FactorPW:
		   if g.pw == nil {
			   return ErrUnset
		  }
		   return g.pw.Verify(ctx, userID, token)
	  case FactorTOTP:
		  return g.verifyTOTP(ctx, userID, token)
	  case FactorEmail, FactorSMS:
		   return g.verifyOTP(ctx, userID, factor, token)
	  default:
		  return ErrFactor
	   }
   }

  func (g *Gate) Issue(ctx context.Context, userID, factor string) (Challenge, string, error) {
	  if g == nil || g.kv == nil {
		   return Challenge{}, "", ErrStore
	   }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return Challenge{}, "", ErrUser
	   }
	  factor = strings.ToLower(strings.TrimSpace(factor))
	   if factor != FactorEmail && factor != FactorSMS {
		   return Challenge{}, "", ErrFactor
	  }
	   if err := g.assertUnlocked(ctx, userID); err != nil {
		  return Challenge{}, "", err
	   }
	   code, err := numericCode(6)
	  if err != nil {
		  return Challenge{}, "", err
	   }
	  salt, err := randomHex(8)
	  if err != nil {
		   return Challenge{}, "", err
	   }
	   ch := Challenge{
		  ID: userID + ":" + factor, UserID: userID, Factor: factor,
		   ExpiresAt: time.Now().UTC().Add(otpTTL).Unix(),
	  }
	  body, _ := json.Marshal(map[string]string{
		   "salt": salt, "hash": hashPIN(salt, code), "exp": fmt.Sprintf("%d", ch.ExpiresAt),
	   })
	   cur, err := g.kv.Get(ctx, kindOTP, ch.ID)
	  exp := persist.Fence{}
	  if err == nil {
		   exp = cur.Fence
	  } else if !errors.Is(err, persist.ErrNotFound) {
		  return Challenge{}, "", err
	   }
	  if err := g.kv.Put(ctx, persist.Row{Kind: kindOTP, Key: ch.ID, Body: body}, exp); err != nil {
		   return Challenge{}, "", err
	  }
	   if g.send != nil {
		   if err := g.send(ctx, userID, factor, code); err != nil {
			  return Challenge{}, "", err
		  }
	   }
	  return ch, code, nil
   }

  func (g *Gate) BindTOTP(ctx context.Context, userID, secret string) error {
	  if g == nil || g.kv == nil {
		   return ErrStore
	  }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return ErrUser
	  }
	  secret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	   if secret == "" {
		  raw, err := randomHex(10)
		   if err != nil {
			  return err
		  }
		   secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(raw))
	   }
	   body, _ := json.Marshal(map[string]string{"secret": secret})
	   cur, err := g.kv.Get(ctx, kindTOTP, userID)
	   exp := persist.Fence{}
	   existed := false
	   if err == nil {
		  exp = cur.Fence
		  existed = true
	  } else if !errors.Is(err, persist.ErrNotFound) {
		  return err
	   }
	   if err := g.kv.Put(ctx, persist.Row{Kind: kindTOTP, Key: userID, Body: body}, exp); err != nil {
		   return err
	   }
	  if existed && g.sec != nil {
		  _ = g.sec.MarkGoogle2FA(ctx, userID)
	   }
	  return nil
  }

  func (g *Gate) TOTPSecret(ctx context.Context, userID string) (string, error) {
	   if g == nil || g.kv == nil {
		   return "", ErrStore
	  }
	   row, err := g.kv.Get(ctx, kindTOTP, strings.TrimSpace(userID))
	   if err != nil {
		  return "", ErrUnset
	   }
	   var rec struct {
		   Secret string `json:"secret"`
	  }
	   if json.Unmarshal(row.Body, &rec) != nil || rec.Secret == "" {
		   return "", ErrUnset
	  }
	  return rec.Secret, nil
   }

  func (g *Gate) TOTPBound(ctx context.Context, userID string) bool {
	   _, err := g.TOTPSecret(ctx, userID)
	  return err == nil
   }

  func (g *Gate) verifyOTP(ctx context.Context, userID, factor, code string) error {
	   row, err := g.kv.Get(ctx, kindOTP, userID+":"+factor)
	   if err != nil {
		  return ErrUnset
	  }
	   var rec struct {
		  Salt string `json:"salt"`
		   Hash string `json:"hash"`
		   Exp  string `json:"exp"`
	  }
	   if json.Unmarshal(row.Body, &rec) != nil {
		   return ErrCode
	  }
	   until, err := strconv.ParseInt(rec.Exp, 10, 64)
	  if err != nil {
		   return ErrCode
	   }
	  if until > 0 && time.Now().UTC().Unix() > until {
		   return ErrExpired
	  }
	   if rec.Hash == "used" {
		   return ErrCode
	  }
	  if subtle.ConstantTimeCompare([]byte(hashPIN(rec.Salt, code)), []byte(rec.Hash)) != 1 {
		  return ErrCode
	  }
	   rec.Hash = "used"
	  body, err := json.Marshal(rec)
	   if err != nil {
		   return err
	   }
	   if err := g.kv.Put(ctx, persist.Row{Kind: kindOTP, Key: userID + ":" + factor, Body: body}, row.Fence); err != nil {
		   return err
	   }
	  return nil
  }

   func (g *Gate) verifyTOTP(ctx context.Context, userID, code string) error {
	   secret, err := g.TOTPSecret(ctx, userID)
	   if err != nil {
		   return err
	   }
	  now := time.Now().UTC().Unix() / 30
	  for _, slot := range []int64{now - 1, now, now + 1} {
		  if subtle.ConstantTimeCompare([]byte(hotp(secret, slot)), []byte(strings.TrimSpace(code))) == 1 {
			  return nil
		  }
	   }
	   return ErrCode
  }

   func normalizeCombos(in [][]string) [][]string {
	   if len(in) == 0 {
		  return nil
	   }
	   var out [][]string
	   for _, combo := range in {
		  var row []string
		  seen := map[string]struct{}{}
		  for _, f := range combo {
			   f = strings.ToLower(strings.TrimSpace(f))
			   switch f {
			   case FactorPW, FactorTOTP, FactorEmail, FactorSMS, FactorAllow,
				   "trade_password", "trading_password", "pin", "ga", "2fa", "google_2fa", "phone", "mobile":
				   f = aliasFactor(f)
				  if _, ok := seen[f]; ok || f == "" {
					  continue
				  }
				   seen[f] = struct{}{}
				   row = append(row, f)
			   }
		  }
		  if len(row) > 0 {
			   out = append(out, row)
		  }
	   }
	   return out
  }

   func aliasFactor(f string) string {
	   switch f {
	   case "trade_password", "trading_password", "pin":
		  return FactorPW
	   case "ga", "2fa", "google_2fa":
		   return FactorTOTP
	  case "phone", "mobile":
		   return FactorSMS
	   default:
		  return f
	  }
  }

   func hashPIN(salt, pin string) string {
	  sum := sha256.Sum256([]byte(salt + ":" + pin))
	  return hex.EncodeToString(sum[:])
   }

   func numericCode(n int) (string, error) {
	   buf := make([]byte, n)
	  if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		  return "", err
	   }
	   out := make([]byte, n)
	   for i := 0; i < n; i++ {
		  out[i] = '0' + buf[i]%10
	  }
	   return string(out), nil
  }

  func randomHex(n int) (string, error) {
	   buf := make([]byte, n)
	   if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		  return "", err
	   }
	   return hex.EncodeToString(buf), nil
  }

   func hotp(secret string, counter int64) string {
	  key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	   if err != nil {
		   return ""
	   }
	  var buf [8]byte
	   binary.BigEndian.PutUint64(buf[:], uint64(counter))
	  mac := hmac.New(sha1.New, key)
	   _, _ = mac.Write(buf[:])
	   sum := mac.Sum(nil)
	  off := sum[len(sum)-1] & 0x0f
	   num := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	  return fmt.Sprintf("%06d", num%1000000)
  }
