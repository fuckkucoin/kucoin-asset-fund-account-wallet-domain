package holdcat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TokenState HoldToken 生命周期状态。
type TokenState string

const (
	TokenStateActive    TokenState = "active"
	TokenStateReleasing TokenState = "releasing" // 协议中间态，防并发双释
	TokenStateReleased TokenState = "released"
	TokenStateBroken   TokenState = "broken" // 漂移异常，由 Gap Patrol 标记
)

// HoldToken 冻结令牌，携带层、原因、账本引用。

type HoldToken struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Book   string `json:"book"`
	Amount string `json:"amount"` // 令牌冻结额，十进制字符串
	Layer  string `json:"layer"`  // Layer.Code
	ReasonCode string `json:"reason_code"`
	LedgerRef  string `json:"ledger_ref,omitempty"` // 账本冻结科目引用（如 transfer_id）
	IdemKey    string `json:"idem_key"`
	ApprovalRef string `json:"approval_ref,omitempty"` // 需审批时的审批单 ID
	Note        string `json:"note,omitempty"`
	State     TokenState `json:"state"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
	ReleasedAt string `json:"released_at,omitempty"`
	BrokenAt   string `json:"broken_at,omitempty"`
}

const (
	kindToken    = "aa_hold_token"
	kindTokenRef = "aa_hold_token_ref" // idem_key → token_id
)

// TokenBook HoldToken 持久化账本。

type TokenBook struct {
	kv persist.KV
}

// NewTokenBook 创建 TokenBook；kv 必须非 nil。

   func NewTokenBook(kv persist.KV) *TokenBook {
	   return &TokenBook{kv: kv}
   }

// Place 创建或幂等返回 HoldToken。
// 幂等键：idemKey（非空时）；重复 Place 同 idemKey 返回已有 token。
// 校验：layer 有效、amount > 0、reasonCode 属于 layer 合法集合。

   func (tb *TokenBook) Place(
	  ctx context.Context,
	  userID, asset, book, amount string,
	  layer Layer,
	  reasonCode ReasonCode,
	   ledgerRef, idemKey, approvalRef, note string,
  ) (HoldToken, error) {
	  if tb == nil || tb.kv == nil {
		   return HoldToken{}, ErrStore
	   }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return HoldToken{}, ErrUser
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		  return HoldToken{}, ErrAmount
	  }
	   amt, err := decimal.NewFromString(strings.TrimSpace(amount))
	   if err != nil || !amt.IsPositive() {
		  return HoldToken{}, ErrAmount
	  }
	   if !layer.Valid() {
		  return HoldToken{}, ErrLayerInvalid
	  }
	   if !ValidReasonForLayer(layer, reasonCode) {
		  return HoldToken{},
			   ErrReasonMismatch
	   }
	  if book == "" {
		  book = "funding"
	  }
	  idemKey = strings.TrimSpace(idemKey)
	   if idemKey == "" {
		  return HoldToken{}, ErrIdemKeyRequired
	  }
	  // 幂等：已存在则直接返回
	   if prev, err := tb.byIdem(ctx, idemKey); err == nil {
		   return prev, nil
	   }
	  now := time.Now().UTC().Format(time.RFC3339)
	   tok := HoldToken{
		  ID:     uuid.NewString(),
		   UserID: userID,
		   Asset:      asset,
		   Book:       strings.ToLower(book),
		  Amount:     amt.String(),
		   Layer:      layer.Code(),
		  ReasonCode: string(reasonCode),
		  LedgerRef:   strings.TrimSpace(ledgerRef),
		   IdemKey:     idemKey,
		   ApprovalRef: strings.TrimSpace(approvalRef),
		   Note: strings.TrimSpace(note),
		   State: TokenStateActive,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if err := tb.writeIdemIndex(ctx, idemKey, tok.ID); err != nil {
		   // 幂等竞争：尝试读已存在
		   if prev, e2 := tb.byIdem(ctx, idemKey); e2 == nil {
			   return prev, nil
		   }
		   return HoldToken{}, err
	   }
	  body, err := json.Marshal(tok)
	   if err != nil {
		   return HoldToken{}, err
	  }
	  if err := tb.kv.Put(ctx, persist.Row{Kind: kindToken, Key: tok.ID, Body: body}, persist.Fence{}); err != nil {
		  return HoldToken{}, err
	  }
	  return tok, nil
  }

// Get 按 ID 获取 token，可按 userID 校验归属（空字符串跳过校验）。

   func (tb *TokenBook) Get(ctx context.Context, userID, id string) (HoldToken, error) {
	  tok, _, err := tb.load(ctx, id)
	   if err != nil {
		   return HoldToken{}, err
	   }
	  if userID != "" && tok.UserID != userID {
		   return HoldToken{}, ErrOwner
	   }
	   return tok, nil
   }

// BeginRelease 将 token 置为 releasing 中间态；调用方完成账本解冻后再调用 CommitRelease。
// 防止并发双释：只有 active 态可进入 releasing。
  func (tb *TokenBook) BeginRelease(ctx context.Context, id string) (HoldToken, error) {
	   tok,
		   stored, err := tb.load(ctx, id)
	  if err != nil {
		   return HoldToken{}, err
	   }
	   if tok.State == TokenStateReleased {
		  return tok, nil // 幂等
	   }
	  if tok.State == TokenStateReleasing {
		   return tok, nil // 已在释放中
	  }
	   if tok.State != TokenStateActive {
		   return HoldToken{}, ErrTokenNotActive
	   }
	  tok.State = TokenStateReleasing
	  tok.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	  return tb.save(ctx, tok, stored.Fence)
   }

// CommitRelease 完成释放，写入 released_at。
// 调用前必须已完成账本解冻；从 releasing 到 released。

  func (tb *TokenBook) CommitRelease(ctx context.Context, id string) (HoldToken, error) {
	  tok, stored, err := tb.load(ctx, id)
	  if err != nil {
		   return HoldToken{}, err
	   }
	  if tok.State == TokenStateReleased {
		   return tok, nil // 幂等
	  }
	  if tok.State != TokenStateReleasing {
		  return HoldToken{},
			  ErrTokenNotReleasing
	   }
	   now := time.Now().UTC().Format(time.RFC3339)
	   tok.State = TokenStateReleased
	  tok.ReleasedAt = now
	  tok.UpdatedAt = now
	   return tb.save(ctx, tok, stored.Fence)
  }

// MarkBroken 将 token 标记为 broken；仅 Gap Patrol 调用。
// active/releasing 均可 broken；已 released/broken 幂等返回。

   func (tb *TokenBook) MarkBroken(ctx context.Context, id, reason string) (HoldToken, error) {
	  tok, stored, err := tb.load(ctx, id)
	  if err != nil {
		  return HoldToken{}, err
	   }
	   if tok.State == TokenStateBroken {
		   return tok, nil
	  }
	  if tok.State == TokenStateReleased {
		   return tok, nil
	   }
	   now := time.Now().UTC().Format(time.RFC3339)
	   tok.State = TokenStateBroken
	   tok.BrokenAt = now
	  tok.UpdatedAt = now
	   if reason != "" {
		  tok.Note = appendNote(tok.Note, "broken: "+reason)
	  }
	  return tb.save(ctx, tok, stored.Fence)
   }

// List 按用户列出 token；activeOnly=true 仅返回 active/releasing。

  func (tb *TokenBook) List(ctx context.Context, userID string, activeOnly bool) ([]HoldToken, error) {
	  if tb == nil || tb.kv == nil {
		  return nil, nil
	  }
	  rows, err := tb.kv.List(ctx, kindToken, 1000)
	  if err != nil {
		   return nil, err
	  }
	   out := make([]HoldToken, 0, len(rows))
	  for _, r := range rows {
		   var tok HoldToken
		   if json.Unmarshal(r.Body, &tok) != nil || tok.ID == "" {
			   continue
		  }
		   if userID != "" && tok.UserID != userID {
			   continue
		  }
		  if activeOnly && tok.State != TokenStateActive && tok.State != TokenStateReleasing {
			   continue
		  }
		  out = append(out, tok)
	  }
	   return out, nil
  }

// ListByLayer 返回特定层的 active token（跨用户），供 Gap Patrol 使用。

  func (tb *TokenBook) ListByLayer(ctx context.Context, layer Layer, activeOnly bool) ([]HoldToken, error) {
	   if tb == nil || tb.kv == nil {
		   return nil, nil
	   }
	   rows, err := tb.kv.List(ctx, kindToken, 2000)
	   if err != nil {
		   return nil, err
	   }
	   code := layer.Code()
	   out := make([]HoldToken, 0)
	   for _, r := range rows {
		   var tok HoldToken
		   if json.Unmarshal(r.Body, &tok) != nil || tok.ID == "" {
			   continue
		  }
		  if tok.Layer != code {
			  continue
		   }
		  if activeOnly && tok.State != TokenStateActive && tok.State != TokenStateReleasing {
			   continue
		   }
		   out = append(out, tok)
	   }
	   return out, nil
  }

// HasHigherLayerToken 判断用户/资产/账本是否存在比 target 层更高（或等级相同）的 active token。
// 用于释放前的层权限校验。
  func (tb *TokenBook) HasHigherLayerToken(
	   ctx context.Context,
	   userID, asset, book string,
	   targetLayer Layer,
  ) (bool, Layer, error) {
	  toks, err := tb.List(ctx, userID, true)
	   if err != nil {
		  return false, LayerUnknown, err
	   }
	   highest := LayerUnknown
	   for _, t := range toks {
		  if t.Asset != asset {
			  continue
		  }
		   if t.Book != "" && book != "" && t.Book != book {
			   continue
		  }
		   l, err := ParseLayer(t.Layer)
		  if err != nil {
			  continue
		   }
		   if l.Priority() > highest.Priority() {
			  highest = l
		   }
	   }
	   if highest == LayerUnknown {
		   return false, LayerUnknown, nil
	   }
	  // 存在比 targetLayer 先级更高的 active token
	   blocked := highest.Priority() > targetLayer.Priority()
	   return blocked, highest, nil
  }

// ActiveLayersForUser 返回用户/资产上所有 active token 的去重层集合。
   func (tb *TokenBook) ActiveLayersForUser(ctx context.Context, userID, asset string) ([]Layer, error) {
	  toks, err := tb.List(ctx, userID, true)
	   if err != nil {
		  return nil, err
	   }
	   seen := map[Layer]bool{}
	   for _, t := range toks {
		   if t.Asset != asset {
			   continue
		   }
		   l, err := ParseLayer(t.Layer)
		  if err != nil || !l.Valid() {
			   continue
		  }
		  seen[l] = true
	  }
	  out := make([]Layer, 0, len(seen))
	  for l := range seen {
		  out = append(out, l)
	  }
	   return out, nil
   }

  func (tb *TokenBook) byIdem(ctx context.Context, idemKey string) (HoldToken, error) {
	  r,
		   err := tb.kv.Get(ctx, kindTokenRef, idemKey)
	   if err != nil {
		   return HoldToken{}, err
	   }
	  tok, _, err := tb.load(ctx, string(r.Body))
	  return tok, err
  }

   func (tb *TokenBook) writeIdemIndex(ctx context.Context, idemKey, tokenID string) error {
	  return tb.kv.Put(
		  ctx,
		  persist.Row{Kind: kindTokenRef, Key: idemKey, Body: []byte(tokenID)},
		  persist.Fence{},
	  )
  }

   func (tb *TokenBook) load(ctx context.Context, id string) (HoldToken, persist.Row, error) {
	   if tb == nil || tb.kv == nil {
		   return HoldToken{}, persist.Row{}, ErrStore
	  }
	  stored, err := tb.kv.Get(ctx, kindToken, strings.TrimSpace(id))
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return HoldToken{}, persist.Row{}, ErrGone
		  }
		   return HoldToken{}, persist.Row{}, err
	   }
	  var tok HoldToken
	  if json.Unmarshal(stored.Body, &tok) != nil || tok.ID == "" {
		  return HoldToken{}, persist.Row{}, ErrGone
	   }
	   return tok, stored, nil
  }

   func (tb *TokenBook) save(ctx context.Context, tok HoldToken, fence persist.Fence) (HoldToken, error) {
	   body, err := json.Marshal(tok)
	  if err != nil {
		   return HoldToken{}, err
	   }
	   if err := tb.kv.Put(ctx, persist.Row{Kind: kindToken, Key: tok.ID, Body: body}, fence); err != nil {
		   return HoldToken{}, err
	  }
	   return tok, nil
  }

  func appendNote(existing, extra string) string {
	  if existing == "" {
		   return extra
	   }
	  return existing + "; " + extra
   }
