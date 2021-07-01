package fiatadd

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const kind = "aa_fiat_add"

var (
	ErrUser = errors.New("fiat add user required")
	ErrAmount = errors.New("fiat add amount invalid")
	ErrStore  = errors.New("fiat add store unavailable")
	ErrVendor = errors.New("fiat add vendor unavailable")
	ErrUnsigned = errors.New("fiat add confirmation unsigned")
	ErrState    = errors.New("fiat add session not pending")
	ErrGone = errors.New("fiat add session not found")
	ErrOwner = errors.New("fiat add owner mismatch")
	ErrDest       = errors.New("fiat add vendor url not allowed")
	ErrKey        = errors.New("fiat add vendor key missing")
	ErrConfirming = errors.New("fiat add session already confirming")
	ErrGenBound = errors.New("fiat add signature bound to older generation")
)

// Session 法币上分会话。ConfirmGen 由确认闸在 Open→Confirm CAS 时单调递增；
// Status 状态机：pending → confirming → credited | forked。
// 并发 Confirm 只允许一路进入 confirming；第二路遇 ErrConfirming 后幂等回放。
type Session struct {
	ID string `json:"id"`
	UserID   string `json:"user_id"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	Vendor   string `json:"vendor"`
	Status   string `json:"status"`
	Checkout string `json:"checkout_url,omitempty"`
	CreatedAt      string `json:"created_at"`
	CreditID       string `json:"credit_id,omitempty"`
	ConfirmGen     int64  `json:"confirm_gen,omitempty"`
	ConfirmLeaseID string `json:"confirm_lease_id,omitempty"`
}

type Vendor interface {
	Name() string
	Create(ctx context.Context, sess Session) (Session, error)
}

type Book struct {
	kv persist.KV
	led *ledger.Service
	vendor Vendor
	key    string
}

  func New(kv persist.KV, led *ledger.Service) *Book {
	   return &Book{kv: kv, led: led}
   }

  func (b *Book) BindVendor(v Vendor, key string) *Book {
	  if b != nil {
		  b.vendor = v
		   b.key = strings.TrimSpace(key)
	  }
	  return b
   }

  func (b *Book) Open(ctx context.Context, userID, asset, amount string) (Session, error) {
	  if b == nil || b.kv == nil {
		  return Session{}, ErrStore
	   }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return Session{}, ErrUser
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  amt, err := decimal.NewFromString(strings.TrimSpace(amount))
	   if err != nil || !amt.IsPositive() {
		   return Session{}, ErrAmount
	  }
	  sess := Session{
		  ID: uuid.NewString(), UserID: userID, Asset: asset, Amount: amt.String(),
		  Status: "pending", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	   }
	  if b.vendor != nil {
		   sess.Vendor = b.vendor.Name()
		   got, err := b.vendor.Create(ctx, sess)
		   if err != nil {
			  return Session{}, err
		   }
		   sess = got
	   } else {
		   sess.Vendor = "table"
	   }
	   if err := b.save(ctx, sess, persist.Fence{}); err != nil {
		   return Session{}, err
	  }
	  return sess, nil
  }

  func (b *Book) Confirm(ctx context.Context, userID, id, signature string) (Session, error) {
	   sess, stored, err := b.load(ctx, id)
	  if err != nil {
		   return Session{}, err
	  }
	   if userID != "" && sess.UserID != userID {
		  return Session{}, ErrOwner
	  }
	  if !validSig(b.key, sess.ID, sess.Amount, signature) {
		   return Session{}, ErrUnsigned
	   }
	   if sess.Status == "credited" {
		  return sess, nil
	  }
	  if sess.Status == "confirming" {
		  return Session{}, ErrConfirming
	   }
	  if sess.Status != "pending" {
		   return Session{}, ErrState
	   }
	   if b.led != nil {
		  tr, err := b.led.Create(ctx, ledger.CreateReq{
			   UserID: sess.UserID, Direction: ledger.FiatOnramp, Asset: sess.Asset,
			   Amount: mustDec(sess.Amount), IdempotencyKey: "fiat-add:" + sess.ID,
		   })
		  if err != nil {
			   return Session{}, err
		  }
		  if tr != nil {
			  sess.CreditID = tr.ID
		  }
	  }
	  sess.Status = "credited"
	  if err := b.save(ctx, sess, stored.Fence); err != nil {
		   return Session{}, err
	   }
	  return sess, nil
   }

// ConfirmWithGen 供确认闸调用：CAS pending→confirming，返回抬升后的 Session。
// 调用方持有确认租约后，执行贷记，再调 CommitCredit 完成 confirming→credited。
// confirming 期间并发第二路 Confirm 收到 ErrConfirming，走幂等回放路径。
// 签名校验职责在调用方（Gate）处理：Gate 持有新 gen 后调 ValidateBoundSig；
// 本方法仅检查签名非空（防止无凭据 CAS），不做 HMAC 校验。
  func (b *Book) ConfirmWithGen(ctx context.Context, userID, id, signature, leaseID string) (Session, error) {
	  sess, stored, err := b.load(ctx, id)
	   if err != nil {
		  return Session{}, err
	  }
	  if userID != "" && sess.UserID != userID {
		  return Session{}, ErrOwner
	  }
	   if strings.TrimSpace(signature) == "" {
		   return Session{}, ErrUnsigned
	  }
	   if sess.Status == "credited" {
		   return sess, nil
	   }
	  if sess.Status == "confirming" {
		  return Session{}, ErrConfirming
	  }
	   if sess.Status != "pending" {
		  return Session{}, ErrState
	  }
	   sess.ConfirmGen++
	   sess.ConfirmLeaseID = leaseID
	  sess.Status = "confirming"
	  if err := b.save(ctx, sess, stored.Fence); err != nil {
		   return Session{}, err
	  }
	   return sess, nil
   }

// CommitCredit 在 confirming 状态下，写入 CreditID 并推进到 credited。
// 若 Save 失败，调用方须产出 FiatAddForkCase 以阻止进一步 Confirm。

  func (b *Book) CommitCredit(ctx context.Context, id, creditID string, gen int64) (Session, error) {
	   sess, stored, err := b.load(ctx, id)
	   if err != nil {
		  return Session{}, err
	   }
	  if sess.Status == "credited" && sess.CreditID == creditID {
		  return sess, nil
	   }
	  if sess.Status != "confirming" {
		   return Session{}, ErrState
	   }
	   if sess.ConfirmGen != gen {
		  return Session{}, ErrGenBound
	  }
	   sess.CreditID = creditID
	   sess.Status = "credited"
	  if err := b.save(ctx, sess, stored.Fence); err != nil {
		  return Session{}, err
	   }
	  return sess, nil
  }

// MarkForked 将 confirming 的 Session 标记为 forked（贷记后 Save 失败且 ForkCase 已落）。

   func (b *Book) MarkForked(ctx context.Context, id string, gen int64, creditID string) (Session, error) {
	   sess, stored, err := b.load(ctx, id)
	   if err != nil {
		  return Session{}, err
	  }
	   if sess.Status == "forked" {
		   return sess, nil
	  }
	  if sess.Status != "confirming" {
		   return Session{}, ErrState
	   }
	   if sess.ConfirmGen != gen {
		  return Session{}, ErrGenBound
	   }
	  sess.Status = "forked"
	   sess.CreditID = creditID
	  if err := b.save(ctx, sess, stored.Fence); err != nil {
		   return Session{}, err
	  }
	   return sess, nil
   }

  func (b *Book) Get(ctx context.Context, userID, id string) (Session, error) {
	  sess, _, err := b.load(ctx, id)
	   if err != nil {
		   return Session{}, err
	  }
	   if userID != "" && sess.UserID != userID {
		  return Session{}, ErrOwner
	   }
	   return sess, nil
   }

  func (b *Book) save(ctx context.Context, sess Session, exp persist.Fence) error {
	  body, err := json.Marshal(sess)
	  if err != nil {
		  return err
	  }
	   return b.kv.Put(ctx, persist.Row{Kind: kind, Key: sess.ID, Body: body}, exp)
  }

   func (b *Book) load(ctx context.Context, id string) (Session, persist.Row, error) {
	   if b == nil || b.kv == nil {
		  return Session{}, persist.Row{}, ErrStore
	  }
	   row, err := b.kv.Get(ctx, kind, strings.TrimSpace(id))
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return Session{}, persist.Row{}, ErrGone
		   }
		  return Session{}, persist.Row{}, err
	   }
	   var sess Session
	  if json.Unmarshal(row.Body, &sess) != nil || sess.ID == "" {
		  return Session{}, persist.Row{}, ErrGone
	   }
	  return sess, row, nil
   }

   func Sign(key, id, amount string) string {
	  mac := hmac.New(sha256.New, []byte(key))
	  _, _ = mac.Write([]byte(id + ":" + amount))
	   return hex.EncodeToString(mac.Sum(nil))
   }

  func validSig(key, id, amount, sig string) bool {
	  if strings.TrimSpace(key) == "" || strings.TrimSpace(sig) == "" {
		  return false
	   }
	   want := Sign(key, id, amount)
	   return hmac.Equal([]byte(want), []byte(strings.ToLower(strings.TrimSpace(sig))))
  }

   func mustDec(s string) decimal.Decimal {
	   n, _ := decimal.NewFromString(s)
	   return n
  }

// HTTPVendor 向公开厂商基址建会话。缺密钥或主机不在允许名单则失败。

type HTTPVendor struct {
	NameVal string
	Base   string
	Key    string
	Client *http.Client
}

  func OfficialVendor(name, key string) (*HTTPVendor, error) {
	  base := config.OfficialFiatBase(name)
	  if base == "" {
		  return nil, ErrVendor
	   }
	  if strings.TrimSpace(key) == "" {
		   return nil, ErrKey
	  }
	  if !config.AllowOutbound(base) {
		  return nil, ErrDest
	  }
	   return &HTTPVendor{NameVal: strings.ToLower(name), Base: base, Key: key, Client: &http.Client{Timeout: 3 * time.Second}}, nil
   }

   func (v *HTTPVendor) Name() string {
	   if v == nil {
		   return ""
	  }
	   return v.NameVal
   }

  func (v *HTTPVendor) Create(ctx context.Context, sess Session) (Session, error) {
	  if v == nil || v.Key == "" {
		  return Session{}, ErrKey
	  }
	   if !config.AllowOutbound(v.Base) {
		   return Session{}, ErrDest
	   }
	   payload, _ := json.Marshal(map[string]string{
		  "externalId": sess.ID, "baseCurrency": sess.Asset, "baseCurrencyAmount": sess.Amount,
	  })
	   req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(v.Base, "/")+"/v1/sessions", bytes.NewReader(payload))
	  if err != nil {
		  return Session{}, err
	  }
	  req.Header.Set("Content-Type", "application/json")
	   req.Header.Set("Authorization", "Bearer "+v.Key)
	   resp, err := v.Client.Do(req)
	   if err != nil {
		  return Session{}, ErrVendor
	   }
	  defer resp.Body.Close()
	  raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	   if resp.StatusCode >= 300 {
		  return Session{}, ErrVendor
	   }
	  var out struct {
		  URL string `json:"url"`
		  ID  string `json:"id"`
	   }
	   _ = json.Unmarshal(raw, &out)
	  if out.URL != "" {
		   sess.Checkout = out.URL
	  }
	  return sess, nil
  }

// TableVendor 与无外呼档：不访问公网，仍要求确认签名。

type TableVendor struct{ NameVal string }

  func (v TableVendor) Name() string {
	  if v.NameVal == "" {
		   return "table"
	   }
	  return v.NameVal
  }

   func (v TableVendor) Create(_ context.Context, sess Session) (Session, error) {
	   sess.Vendor = v.Name()
	   return sess, nil
   }
