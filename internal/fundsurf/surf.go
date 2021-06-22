package fundsurf

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/forbidgate"
	"account-assets.kucoin.net/funding/internal/fundread"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/internal/tradepw"
	"account-assets.kucoin.net/funding/internal/wdverify"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	kindForbid = "aa_wd_forbid"
	kindMemo   = "aa_memo_ticket"
	kindDepTo  = "aa_deposit_to"
	kindWLMode   = "aa_wl_mode"
	kindXferPerm = "aa_sub_xfer_perm"
	maxExportWin = 100 * 24 * time.Hour
)

var (
	ErrUser  = errors.New("user_id required")
	ErrStore = errors.New("funding surface store unavailable")
	ErrLedger = errors.New("funding ledger unavailable")
	ErrWindow = errors.New("export window exceeds 100 days")
	ErrMemo   = errors.New("memo recovery fields required")
	ErrGone   = errors.New("ticket not found")
	ErrForbid = errors.New("user forbidden to withdraw")
	ErrChain = errors.New("chain not listed")
	ErrHalt = custodyhalt.ErrHalted
)

type Book struct {
	kv persist.KV
	led *ledger.Service
	cfg     *config.Config
	depRead DepReader
	sec *seclock.Service
	pw *tradepw.Book
	gate *wdverify.Gate
	attribRead AttribReader
	proj *fundread.Service
	forbidGate  *forbidgate.Gate
	forbidDrift *forbidgate.DriftService
}

  func New(kv persist.KV, led *ledger.Service, cfg *config.Config) *Book {
	   return &Book{kv: kv, led: led, cfg: cfg}
   }

type FrozenRow struct {
	Asset string `json:"asset"`
	Book  string `json:"book"`
	Subject string `json:"subject"`
	Frozen string `json:"frozen"`
	Reason string `json:"reason"`
	Ref    string `json:"ref,omitempty"`
	Category string `json:"category,omitempty"`
}

type LockView struct {
	Locked bool `json:"locked"`
	Until     string `json:"until,omitempty"`
	RemainSec int64  `json:"remainSeconds"`
	Reason    string `json:"reason,omitempty"`
}

type Network struct {
	Chain string `json:"chain"`
	ChainName       string `json:"chainName"`
	Currency        string `json:"currency"`
	ContractAddress string `json:"contractAddress,omitempty"`
	Explorer        string `json:"explorer,omitempty"`
	NeedMemo bool `json:"needMemo"`
	TagName string `json:"tagName,omitempty"` // memo / destination_tag / comment
	MinSize        string `json:"withdrawMinSize"`
	DepositMinSize string `json:"depositMinSize"`
	IsWithdrawOn   bool   `json:"isWithdrawEnabled"`
	IsDepositOn bool `json:"isDepositEnabled"`
}

type MemoTicket struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Memo   string `json:"memo"`
	Address   string `json:"address"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

  func (b *Book) Frozen(ctx context.Context, userID string) ([]FrozenRow, error) {
	  if b == nil || b.led == nil {
		  return nil, ErrLedger
	   }
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return nil, ErrUser
	  }
	  books, err := b.led.Accounts(ctx, userID)
	   if err != nil {
		   return nil, err
	   }
	  var holds []holdcat.Row
	  if b.kv != nil {
		  if rows, err := holdcat.New(b.kv).List(ctx, userID, true); err == nil {
			   holds = rows
		   }
	  }
	  openReason := b.openTransferReasons(ctx, userID)
	  used := map[string]struct{}{}
	   out := make([]FrozenRow, 0)
	  for _, row := range books {
		   amt, err := decimal.NewFromString(row.Frozen)
		   if err != nil || !amt.IsPositive() {
			   continue
		  }
		   reason := freezeReason(row.Book)
		   ref, cat := "", ""
		  key := strings.ToUpper(row.Asset) + "|" + strings.ToLower(row.Book)
		  if h, ok := firstHold(holds, row.Asset, row.Book); ok {
			   reason, ref, cat = h.Reason, h.Ref, h.Reason
			   used[h.ID] = struct{}{}
		  } else if r, ok := openReason[key]; ok {
			  reason, ref, cat = r.Reason, r.Ref, r.Reason
		   }
		   out = append(out, FrozenRow{
			  Asset: row.Asset,
			   Book:  row.Book, Subject: row.Subject,
			   Frozen: row.Frozen, Reason: reason, Ref: ref, Category: cat,
		   })
	   }
	  seenPark := map[string]struct{}{}
	   for _, h := range holds {
		   if _, ok := used[h.ID]; ok {
			   continue
		  }
		  if holdcat.ParkedOnBridge(h.Reason) {
			   // 借记已入 bridge，用户 Frozen=0；展示占用不得再记一笔 funding 冻结。
			  out = append(out, FrozenRow{
				   Asset: h.Asset, Book: "bridge", Subject: "parked",
				  Frozen: h.Amount, Reason: h.Reason, Ref: h.Ref, Category: h.Reason,
			   })
			  seenPark[strings.ToUpper(h.Asset)+"|"+h.Reason] = struct{}{}
			   continue
		  }
		  out = append(out, FrozenRow{
			   Asset: h.Asset,
			   Book:  h.Book, Subject: h.Book,
			   Frozen: h.Amount, Reason: h.Reason, Ref: h.Ref, Category: h.Reason,
		  })
	  }
	   out = append(out, b.parkedLedgerRows(ctx, userID, seenPark)...)
	  return out, nil
   }

  func (b *Book) parkedLedgerRows(ctx context.Context, userID string, seen map[string]struct{}) []FrozenRow {
	  if b == nil || b.led == nil {
		  return nil
	   }
	   list, err := b.led.List(ctx, userID, 40)
	   if err != nil {
		   return nil
	   }
	  out := make([]FrozenRow, 0)
	  for _, tr := range list {
		   if tr == nil || tr.State != ledger.StateCredited {
			  continue
		  }
		  reason := ""
		   switch tr.Direction {
		   case ledger.FiatOfframp:
			  reason = "fiat_offramp"
		  case ledger.SubDebit:
			   reason = "sub_debit"
		  default:
			   continue
		  }
		  k := strings.ToUpper(tr.Asset) + "|" + reason
		  if _, ok := seen[k]; ok {
			   continue
		   }
		   seen[k] = struct{}{}
		   out = append(out, FrozenRow{
			   Asset: tr.Asset, Book: "bridge", Subject: "parked",
			  Frozen: tr.Amount.String(), Reason: reason, Ref: tr.ID, Category: reason,
		  })
	  }
	  return out
   }

  func firstHold(rows []holdcat.Row, asset, book string) (holdcat.Row, bool) {
	  for _, h := range rows {
		  if strings.EqualFold(h.Asset, asset) && strings.EqualFold(h.Book, book) {
			   return h, true
		   }
	   }
	  return holdcat.Row{}, false
  }

   func (b *Book) openTransferReasons(ctx context.Context, userID string) map[string]FrozenRow {
	   out := map[string]FrozenRow{}
	   if b == nil || b.led == nil {
		  return out
	  }
	  list, err := b.led.List(ctx, userID, 0)
	   if err != nil {
		  return out
	   }
	   for _, tr := range list {
		  if tr == nil || !tr.State.Open() || !tr.Direction.Freezes() {
			  continue
		  }
		   reason := freezeReasonFor(tr.Direction)
		  book := string(tr.Direction.SourceBook())
		   key := strings.ToUpper(tr.Asset) + "|" + book
		   if _, exists := out[key]; exists {
			   continue
		  }
		  out[key] = FrozenRow{Asset: tr.Asset,
			  Book: book, Reason: reason, Ref: tr.ID, Category: reason}
	   }
	   return out
   }

  func freezeReasonFor(d ledger.Direction) string {
	  switch d {
	  case ledger.WalletWithdraw, ledger.FiatOfframp, ledger.ConvertDebit, ledger.P2PDebit, ledger.SubDebit:
		  return holdcat.ReasonPendingWithdraw
	   case ledger.EarnSubscribe, ledger.EarnRedeem:
		   return holdcat.ReasonEarnHold
	  case ledger.FundingToMargin, ledger.MarginToFunding:
		  return holdcat.ReasonMarginHold
	   case ledger.FundingToIsolated,
		   ledger.IsolatedToFunding:
		  return holdcat.ReasonIsolatedHold
	  case ledger.FundingToFutures, ledger.FuturesToFunding:
		   return holdcat.ReasonFuturesMargin
	   case ledger.FundingToTrading,
		  ledger.TradingToFunding:
		  return holdcat.ReasonOpenOrders
	  default:
		   return holdcat.ReasonPendingTransfer
	  }
   }

   func freezeReason(book string) string {
	   switch strings.ToLower(strings.TrimSpace(book)) {
	  case "wallet", "spot":
		   return "pending_withdraw_or_transfer"
	  case "trading":
		  return "open_orders"
	   case "margin", "isolated":
		  return "margin_hold"
	  case "futures":
		   return "futures_margin"
	  case "earn":
		   return "earn_hold"
	  default:
		   return "hold"
	  }
   }

// Lock
// 查询用户出金冷却锁状态（倒计时与原因）。
// 每次直读账本冷却水位，组装 Locked/Until/RemainSec/Reason。
// 不得用进程内 Cache 缓存锁态；缓存会导致冷却已生效仍放行出金。
   func (b *Book) Lock(ctx context.Context, userID string) (LockView, error) {
	  if b == nil || b.led == nil {
		  return LockView{}, ErrLedger
	   }
	   until, reason, err := b.led.WithdrawLockUntil(ctx, userID)
	   if err != nil {
		   return LockView{}, err
	   }
	  if until.IsZero() || time.Now().UTC().After(until) {
		   return LockView{Locked: false}, nil
	  }
	   remain := int64(time.Until(until).Seconds())
	   if remain < 0 {
		  remain = 0
	  }
	  return LockView{
		  Locked: true,
		  Until:  until.UTC().Format(time.RFC3339),
		   RemainSec: remain, Reason: reason,
	   }, nil
  }

  func (b *Book) SetForbidden(ctx context.Context, userID string, forbidden bool) error {
	   if b == nil || b.kv == nil {
		   return ErrStore
	  }
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return ErrUser
	   }
	  body, _ := json.Marshal(map[string]bool{"forbidden": forbidden})
	   cur,
		   err := b.kv.Get(ctx, kindForbid, userID)
	   exp := persist.Fence{}
	   if err == nil {
		  exp = cur.Fence
	   } else if !errors.Is(err, persist.ErrNotFound) {
		   return err
	  }
	  return b.kv.Put(ctx, persist.Row{Kind: kindForbid, Key: userID, Body: body}, exp)
   }

  func (b *Book) Forbidden(ctx context.Context, userID string) (bool, error) {
	  if b == nil || b.kv == nil {
		   return false, nil
	   }
	   row, err := b.kv.Get(ctx, kindForbid, strings.TrimSpace(userID))
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return false, nil
		  }
		  return false, err
	   }
	  var rec struct {
		  Forbidden bool `json:"forbidden"`
	  }
	   _ = json.Unmarshal(row.Body, &rec)
	   return rec.Forbidden, nil
  }

// RequireWithdraw 出金前闸：热异常停提、用户禁提、安全变更冷却。
// 冷却经 seclock.RequireWithdraw；未绑 sec 时回退 ledger.WithdrawLockUntil。

   func (b *Book) RequireWithdraw(ctx context.Context, userID string) error {
	  if b != nil && b.kv != nil {
		   if err := custodyhalt.New(b.kv).CheckActive(ctx); err != nil {
			  return err
		   }
	  }
	  ok,
		   err := b.Forbidden(ctx, userID)
	   if err != nil {
		  return err
	  }
	  if ok {
		  return ErrForbid
	  }
	  if b != nil && b.sec != nil {
		   if err := b.sec.RequireWithdraw(ctx, userID); err != nil {
			  return err
		   }
	  } else if b != nil && b.led != nil {
		   until, reason, err := b.led.WithdrawLockUntil(ctx, userID)
		  if err != nil {
			   return err
		   }
		   if !until.IsZero() && until.After(time.Now().UTC()) {
			   if strings.TrimSpace(reason) == "" {
				   reason = seclock.ReasonSecurityChange
			  }
			   return fmt.Errorf("%w: until %s reason %s", ledger.ErrWithdrawCooling, until.UTC().Format(time.RFC3339), reason)
		   }
	  }
	  return nil
  }

// RequireDeposit 热异常停充：与出金共用 custodyhalt；不做用户禁提判断。
   func (b *Book) RequireDeposit(ctx context.Context) error {
	   if b == nil || b.kv == nil {
		  return nil
	  }
	  return custodyhalt.New(b.kv).CheckActive(ctx)
  }

// HaltGate 暴露热钱包异常停提闸，供 ops / recon / withdraw 复用同一 KV。

  func (b *Book) HaltGate() *custodyhalt.Gate {
	   if b == nil {
		  return custodyhalt.New(nil)
	  }
	   return custodyhalt.New(b.kv)
  }

  func (b *Book) SetDepositTo(ctx context.Context, userID, chain, book string) error {
	  if b == nil || b.kv == nil {
		  return ErrStore
	   }
	   userID = strings.TrimSpace(userID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  book = strings.ToUpper(strings.TrimSpace(book))
	   if userID == "" || chain == "" {
		  return ErrUser
	   }
	  if book == "" {
		   book = "MAIN"
	   }
	  body, _ := json.Marshal(map[string]string{"book": book})
	  key := userID + "|" + chain
	   cur, err := b.kv.Get(ctx, kindDepTo, key)
	  exp := persist.Fence{}
	  if err == nil {
		   exp = cur.Fence
	  } else if !errors.Is(err, persist.ErrNotFound) {
		  return err
	   }
	  return b.kv.Put(ctx, persist.Row{Kind: kindDepTo, Key: key, Body: body}, exp)
  }

  func (b *Book) DepositTo(ctx context.Context, userID, chain string) string {
	  if b == nil || b.kv == nil {
		   return "MAIN"
	   }
	  row, err := b.kv.Get(ctx, kindDepTo, strings.TrimSpace(userID)+"|"+strings.ToUpper(strings.TrimSpace(chain)))
	  if err != nil {
		  return "MAIN"
	  }
	  var rec struct {
		  Book string `json:"book"`
	  }
	   _ = json.Unmarshal(row.Body, &rec)
	  if rec.Book == "" {
		  return "MAIN"
	   }
	   return rec.Book
  }

   func (b *Book) SetWhitelistOnly(ctx context.Context, userID string, on bool) error {
	   if b == nil || b.kv == nil {
		   return ErrStore
	   }
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return ErrUser
	  }
	  body, _ := json.Marshal(map[string]bool{"on": on})
	  cur, err := b.kv.Get(ctx, kindWLMode, userID)
	  exp := persist.Fence{}
	  if err == nil {
		  exp = cur.Fence
	   } else if !errors.Is(err, persist.ErrNotFound) {
		  return err
	  }
	   return b.kv.Put(ctx, persist.Row{Kind: kindWLMode, Key: userID, Body: body}, exp)
   }

  func (b *Book) WhitelistOnly(ctx context.Context, userID string) bool {
	  if b == nil || b.kv == nil {
		   return false
	   }
	  row, err := b.kv.Get(ctx, kindWLMode, strings.TrimSpace(userID))
	   if err != nil {
		   return false
	  }
	  var rec struct {
		  On bool `json:"on"`
	  }
	  _ = json.Unmarshal(row.Body, &rec)
	  return rec.On
  }

  func (b *Book) SetSubXferPerm(ctx context.Context, parent, sub string, transferable bool) error {
	  if b == nil || b.kv == nil {
		   return ErrStore
	  }
	   parent, sub = strings.TrimSpace(parent), strings.TrimSpace(sub)
	  if parent == "" || sub == "" {
		  return ErrUser
	  }
	   body, _ := json.Marshal(map[string]bool{"transferable": transferable})
	   key := parent + "|" + sub
	   cur, err := b.kv.Get(ctx, kindXferPerm, key)
	   exp := persist.Fence{}
	  if err == nil {
		   exp = cur.Fence
	  } else if !errors.Is(err, persist.ErrNotFound) {
		   return err
	  }
	  return b.kv.Put(ctx, persist.Row{Kind: kindXferPerm, Key: key, Body: body}, exp)
  }

  func (b *Book) SubXferPerm(ctx context.Context, parent, sub string) bool {
	  if b == nil || b.kv == nil {
		   return true
	   }
	  row, err := b.kv.Get(ctx, kindXferPerm, strings.TrimSpace(parent)+"|"+strings.TrimSpace(sub))
	   if err != nil {
		  return true
	  }
	  var rec struct {
		   Transferable bool `json:"transferable"`
	   }
	  _ = json.Unmarshal(row.Body, &rec)
	   return rec.Transferable
  }

   func (b *Book) OpenNetworkTicket(ctx context.Context, userID, chain, hash, expect, addr string) (MemoTicket, error) {
	  if b == nil || b.kv == nil {
		   return MemoTicket{}, ErrStore
	   }
	   userID = strings.TrimSpace(userID)
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  expect = strings.ToUpper(strings.TrimSpace(expect))
	  hash = strings.TrimSpace(hash)
	   if userID == "" || chain == "" || hash == "" || expect == "" {
		  return MemoTicket{}, ErrMemo
	  }
	  t := MemoTicket{
		   ID: uuid.NewString(), UserID: userID, Chain: chain, TxHash: hash,
		  Memo: expect, Address: strings.TrimSpace(addr), Status: "pending",
		  CreatedAt: time.Now().UTC().Format(time.RFC3339),
	   }
	   body, _ := json.Marshal(t)
	  if err := b.kv.Put(ctx, persist.Row{Kind: "aa_net_ticket", Key: t.ID, Body: body}, persist.Fence{}); err != nil {
		   return MemoTicket{}, err
	  }
	   return t, nil
  }

  func (b *Book) GetNetworkTicket(ctx context.Context, id string) (MemoTicket, error) {
	   if b == nil || b.kv == nil {
		  return MemoTicket{}, ErrStore
	  }
	  row, err := b.kv.Get(ctx, "aa_net_ticket", strings.TrimSpace(id))
	  if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			  return MemoTicket{}, ErrGone
		  }
		   return MemoTicket{}, err
	  }
	   var t MemoTicket
	  if err := json.Unmarshal(row.Body, &t); err != nil || t.ID == "" {
		  return MemoTicket{}, ErrGone
	   }
	   return t, nil
   }

   func (b *Book) OpenMemoTicket(ctx context.Context, userID, chain, hash, memo, addr string) (MemoTicket, error) {
	  if b == nil || b.kv == nil {
		  return MemoTicket{}, ErrStore
	  }
	  userID = strings.TrimSpace(userID)
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   hash = strings.TrimSpace(hash)
	  memo = strings.TrimSpace(memo)
	  if userID == "" || chain == "" || hash == "" || memo == "" {
		  return MemoTicket{}, ErrMemo
	   }
	  t := MemoTicket{
		   ID: uuid.NewString(), UserID: userID, Chain: chain, TxHash: hash,
		  Memo: memo, Address: strings.TrimSpace(addr), Status: "pending",
		   CreatedAt: time.Now().UTC().Format(time.RFC3339),
	   }
	   body, _ := json.Marshal(t)
	  if err := b.kv.Put(ctx, persist.Row{Kind: kindMemo, Key: t.ID, Body: body}, persist.Fence{}); err != nil {
		  return MemoTicket{}, err
	   }
	   return t, nil
  }

  func (b *Book) GetMemoTicket(ctx context.Context, id string) (MemoTicket, error) {
	   if b == nil || b.kv == nil {
		  return MemoTicket{}, ErrStore
	   }
	   row, err := b.kv.Get(ctx, kindMemo, strings.TrimSpace(id))
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return MemoTicket{}, ErrGone
		  }
		   return MemoTicket{}, err
	  }
	   var t MemoTicket
	  if err := json.Unmarshal(row.Body, &t); err != nil {
		  return MemoTicket{}, ErrGone
	   }
	  return t, nil
   }

  func (b *Book) DecideMemo(ctx context.Context,
	  id, status string) (MemoTicket, error) {
	  t, err := b.GetMemoTicket(ctx, id)
	   if err != nil {
		   return MemoTicket{}, err
	  }
	   status = strings.ToLower(strings.TrimSpace(status))
	  if status != "approved" && status != "rejected" {
		  return MemoTicket{}, ErrMemo
	  }
	   t.Status = status
	   body, _ := json.Marshal(t)
	  row, err := b.kv.Get(ctx, kindMemo, t.ID)
	  if err != nil {
		  return MemoTicket{}, err
	   }
	   if err := b.kv.Put(ctx, persist.Row{Kind: kindMemo, Key: t.ID, Body: body}, row.Fence); err != nil {
		   return MemoTicket{}, err
	  }
	   return t, nil
   }

   func (b *Book) Networks(currency string) []Network {
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	   min := "0.00000001"
	  if b != nil && b.cfg != nil && b.cfg.Risk.MinAmount != "" {
		   min = b.cfg.Risk.MinAmount
	   }
	   out := make([]Network, 0)
	   if b == nil || b.cfg == nil {
		  return out
	  }
	  for chain, spec := range b.cfg.Chains {
		  chain = strings.ToUpper(strings.TrimSpace(chain))
		   if chain == "" {
			   continue
		   }
		  n := Network{
			  Chain: chain, ChainName: chain, Currency: currency,
			   Explorer: spec.Explorer,
			  NeedMemo: NeedsMemo(chain), TagName: MemoTagName(chain),
			   MinSize: min, DepositMinSize: min, IsWithdrawOn: len(spec.Endpoints) > 0, IsDepositOn: len(spec.Endpoints) > 0,
		  }
		  if currency != "" && spec.Tokens != nil {
			   if tok, ok := spec.Tokens[currency]; ok {
				   n.ContractAddress = tok.Contract
			  }
		  }
		   if currency != "" && currency != chain && (spec.Tokens == nil || spec.Tokens[currency].Contract == "") && !nativeOf(chain, currency) {
			   continue
		  }
		  out = append(out, n)
	   }
	  return out
  }

  func nativeOf(chain, currency string) bool {
	  switch chain {
	  case "ETH", "ARBITRUM", "OPTIMISM", "BASE":
		  return currency == "ETH"
	  case "BTC":
		   return currency == "BTC"
	   case "SOL":
		   return currency == "SOL"
	  case "TRON":
		   return currency == "TRX"
	   case "TON":
		   return currency == "TON"
	  case "POLYGON":
		   return currency == "MATIC" || currency == "POL"
	   default:
		   return chain == currency
	  }
  }

  func NeedsMemo(chain string) bool {
	   return MemoTagName(chain) != ""
   }

// MemoTagName 公开备注字段名（与 chainad.MemoSpec / 充值页一致）。
   func MemoTagName(chain string) string {
	   switch strings.ToUpper(strings.TrimSpace(chain)) {
	  case "XRP":
		   return "destination_tag"
	  case "TON":
		  return "comment"
	   case "EOS", "ATOM", "XLM":
		  return "memo"
	  default:
		   return ""
	   }
   }

// MemoFor
// 按用户与链派生稳定充值 Memo，供充值页展示。
// 同一用户同一链结果稳定可复现。
// 不得在代码中固化账号或 Memo；派生失败须显式返回空或错误。
   func MemoFor(userID, chain string) string {
	   if !NeedsMemo(chain) {
		  return ""
	  }
	  sum := sha256.Sum256([]byte(strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(chain))))
	   // XRP destination_tag 须 uint32 十进制；其他备注链用稳定 hex。
	   if MemoTagName(chain) == "destination_tag" {
		  n := binary.BigEndian.Uint32(sum[:4])
		   return strconv.FormatUint(uint64(n),
			   10)
	   }
	  return hex.EncodeToString(sum[:8])
  }

   func CheckExportWindow(start, end time.Time) error {
	   if start.IsZero() || end.IsZero() {
		  return nil
	  }
	   if end.Before(start) || end.Sub(start) > maxExportWin {
		  return ErrWindow
	  }
	  return nil
   }
