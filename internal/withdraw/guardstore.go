// 功能：出金守恒与刷提落库。
// 阻断、热意图、刷提跳闸、live hash 拒绝和半冻结引用写入 KV，启动时回填。重启后仍只拦同用户同资产的新出金；跳闸记录本身不解冻。回填或写入失败必须失败即关，不能当成没有阻断。

// 作者：Draven
// 日期：2021.01.13

package withdraw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindWDAbuse = "aa_wd_abuse"
	kindWDBlock = "aa_wd_conserve_block"
	kindWDIntent = "aa_wd_hot_intent"
	kindWDTicket = "aa_wd_conserve_ticket"
	kindWDDeny   = "aa_wd_live_deny"
	kindWDManual = "aa_wd_manual_advance"
	kindWDIdem   = "aa_wd_idem_hold"
	kindWDPhase = "aa_wd_freeze_phase"
)

   func guardKey(s string) string {
	   if len(s) <= 180 {
		   return s
	   }
	  sum := sha256.Sum256([]byte(s))
	  return hex.EncodeToString(sum[:])
   }

  func kvList(ctx context.Context, kv persist.KV, kind string) ([]persist.Row, error) {
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   var out []persist.Row
	   after := ""
	   for page := 0; page < 50; page++ {
		   rows, err := kv.ListAfter(ctx, kind, after, 200)
		   if err != nil {
			  return nil, err
		   }
		  if len(rows) == 0 {
			  return out, nil
		  }
		  out = append(out, rows...)
		  after = rows[len(rows)-1].Key
		  if len(rows) < 200 {
			  return out, nil
		   }
	  }
	  return nil, fmt.Errorf("withdraw guard list truncated")
  }

  func kvPut(ctx context.Context, kv persist.KV, kind, key string, body []byte) error {
	  if kv == nil {
		  return nil
	   }
	   key = guardKey(key)
	   if key == "" || kind == "" {
		  return persist.ErrConflict
	  }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  var last error
	  for i := 0; i < 2; i++ {
		   cur, err := kv.Get(ctx, kind, key)
		   exp := persist.Fence{}
		  if err == nil {
			  exp = cur.Fence
		   } else if !errors.Is(err, persist.ErrNotFound) {
			   return err
		   }
		   last = kv.Put(ctx, persist.Row{Kind: kind, Key: key, Body: body}, exp)
		   if last == nil || !errors.Is(last, persist.ErrStaleFence) {
			  return last
		  }
	  }
	   return last
   }

  func kvDel(ctx context.Context, kv persist.KV, kind, key string) error {
	  if kv == nil {
		  return nil
	   }
	  key = guardKey(key)
	  if ctx == nil {
		   ctx = context.Background()
	   }
	   cur, err := kv.Get(ctx, kind, key)
	  if errors.Is(err, persist.ErrNotFound) {
		  return nil
	  }
	   if err != nil {
		  return err
	  }
	  err = kv.Delete(ctx, kind, key, cur.Fence)
	  if errors.Is(err, persist.ErrNotFound) {
		  return nil
	  }
	   return err
  }

  func (c *Conserve) BindKV(kv persist.KV) error {
	  if c == nil || kv == nil {
		   return nil
	   }
	   c.mu.Lock()
	  c.kv = kv
	   c.mu.Unlock()
	   return c.hydrate(context.Background())
  }

   func (c *Conserve) boundKV() persist.KV {
	  if c == nil {
		  return nil
	   }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	  return c.kv
   }

   func (c *Conserve) setStoreErr(err error) {
	   if c == nil {
		   return
	  }
	   c.mu.Lock()
	   c.storeErr = err
	  c.mu.Unlock()
   }

  func (c *Conserve) readStoreErr() error {
	   if c == nil {
		   return nil
	  }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  return c.storeErr
  }

   func (c *Conserve) hydrate(ctx context.Context) error {
	   kv := c.boundKV()
	   if kv == nil {
		  return nil
	  }
	  blocks, err := kvList(ctx, kv, kindWDBlock)
	   if err != nil {
		  return err
	   }
	  intents, err := kvList(ctx, kv, kindWDIntent)
	   if err != nil {
		  return err
	   }
	  marks,
		  err := kvList(ctx, kv, kindWDTicket)
	  if err != nil {
		  return err
	  }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   for _, row := range blocks {
		   var b Block
		  if err := json.Unmarshal(row.Body, &b); err != nil {
			   return fmt.Errorf("conserve block decode: %w", err)
		   }
		  if b.UserID == "" || b.Asset == "" {
			  continue
		   }
		   c.blocks[pairKey(b.UserID, b.Asset)] = b
	  }
	   for _, row := range intents {
		  var in HotIntent
		   if err := json.Unmarshal(row.Body, &in); err != nil {
			  return fmt.Errorf("hot intent decode: %w", err)
		  }
		   if in.TransferID == "" {
			   continue
		   }
		   c.intents[in.TransferID] = in
	   }
	   for _, row := range marks {
		  var mark struct {
			  ID string `json:"id"`
		  }
		   if err := json.Unmarshal(row.Body, &mark); err != nil {
			  return fmt.Errorf("conserve ticket mark decode: %w", err)
		   }
		   if row.Key != "" {
			  c.ticketed[row.Key] = mark.ID
		   }
	  }
	   c.storeErr = nil
	  return nil
   }

  func (c *Conserve) saveBlock(b Block) error {
	   body, err := json.Marshal(b)
	   if err != nil {
		   return err
	   }
	  return kvPut(context.Background(),
		   c.boundKV(), kindWDBlock, pairKey(b.UserID, b.Asset), body)
   }

  func (c *Conserve) syncBlocks(next map[string]Block) error {
	   kv := c.boundKV()
	   if kv == nil {
		   return nil
	   }
	  ctx := context.Background()
	  have, err := kvList(ctx, kv, kindWDBlock)
	  if err != nil {
		  return err
	   }
	  seen := map[string]struct{}{}
	   for k, b := range next {
		  if err := c.saveBlock(b); err != nil {
			   return err
		   }
		  seen[guardKey(k)] = struct{}{}
	   }
	  for _, row := range have {
		   if _, ok := seen[row.Key]; ok {
			   continue
		  }
		   if err := kvDel(ctx, kv, kindWDBlock, row.Key); err != nil {
			  return err
		   }
	   }
	  return nil
   }

  func (c *Conserve) saveIntent(in HotIntent) error {
	   body, err := json.Marshal(in)
	  if err != nil {
		   return err
	  }
	  return kvPut(context.Background(),
		  c.boundKV(), kindWDIntent, in.TransferID, body)
   }

   func (c *Conserve) deleteIntent(id string) error {
	  return kvDel(context.Background(), c.boundKV(), kindWDIntent, id)
   }

  func (c *Conserve) saveTicketMark(key,
	  id string) error {
	   body, err := json.Marshal(struct {
		  ID string `json:"id"`
	  }{ID: id})
	   if err != nil {
		  return err
	   }
	  return kvPut(context.Background(), c.boundKV(), kindWDTicket, key, body)
   }

   func (a *Abuse) BindKV(kv persist.KV) error {
	  if a == nil || kv == nil {
		  return nil
	  }
	   a.mu.Lock()
	   a.kv = kv
	   a.mu.Unlock()
	   rows, err := kvList(context.Background(), kv, kindWDAbuse)
	  if err != nil {
		  return err
	  }
	   a.mu.Lock()
	  defer a.mu.Unlock()
	  for _, row := range rows {
		   var hit AbuseHit
		   if err := json.Unmarshal(row.Body, &hit); err != nil {
			   return fmt.Errorf("abuse hit decode: %w", err)
		  }
		   if hit.UserID == "" || hit.Asset == "" {
			   continue
		   }
		   a.hits[pairKey(hit.UserID, hit.Asset)] = hit
	   }
	   return nil
  }

   func (a *Abuse) saveHit(hit AbuseHit) error {
	   if a == nil {
		   return nil
	   }
	  a.mu.Lock()
	  kv := a.kv
	  a.mu.Unlock()
	  if kv == nil {
		   return nil
	   }
	  body, err := json.Marshal(hit)
	  if err != nil {
		  return err
	   }
	  return kvPut(context.Background(),
		  kv, kindWDAbuse, pairKey(hit.UserID, hit.Asset), body)
  }

   func (g *auditLog) BindKV(kv persist.KV) error {
	  if g == nil || kv == nil {
		  return nil
	   }
	   g.mu.Lock()
	   g.kv = kv
	  g.mu.Unlock()
	   denies, err := kvList(context.Background(), kv, kindWDDeny)
	  if err != nil {
		  return err
	  }
	   manuals, err := kvList(context.Background(), kv, kindWDManual)
	  if err != nil {
		   return err
	  }
	  g.mu.Lock()
	   defer g.mu.Unlock()
	   for _, row := range denies {
		  var d LiveDeny
		   if err := json.Unmarshal(row.Body, &d); err != nil {
			   return fmt.Errorf("live deny decode: %w", err)
		   }
		  g.liveDeny = append(g.liveDeny, d)
	   }
	  for _, row := range manuals {
		  var m ManualRecord
		   if err := json.Unmarshal(row.Body, &m); err != nil {
			  return fmt.Errorf("manual advance decode: %w", err)
		  }
		   g.manual = append(g.manual, m)
	   }
	   return nil
   }

  func (g *auditLog) saveDeny(d LiveDeny) error {
	  if g == nil {
		  return nil
	   }
	   g.mu.Lock()
	  kv := g.kv
	   g.mu.Unlock()
	   if kv == nil {
		   return nil
	  }
	  body, err := json.Marshal(d)
	  if err != nil {
		  return err
	   }
	   key := d.At.UTC().Format("20060102T150405.000000000") + "|" + guardKey(d.TransferID)
	  return kvPut(context.Background(), kv, kindWDDeny, key, body)
  }

  func (g *auditLog) saveManual(m ManualRecord) error {
	   if g == nil {
		  return nil
	  }
	   g.mu.Lock()
	  kv := g.kv
	   g.mu.Unlock()
	   if kv == nil {
		   return nil
	  }
	  body, err := json.Marshal(m)
	   if err != nil {
		  return err
	   }
	  key := m.At.UTC().Format("20060102T150405.000000000") + "|" + guardKey(m.TransferID)
	  return kvPut(context.Background(), kv, kindWDManual, key, body)
   }

  func (x *idemIndex) BindKV(kv persist.KV) error {
	  if x == nil || kv == nil {
		   return nil
	   }
	   x.mu.Lock()
	   x.kv = kv
	   x.mu.Unlock()
	   rows, err := kvList(context.Background(), kv, kindWDIdem)
	  if err != nil {
		   return err
	  }
	  x.mu.Lock()
	  defer x.mu.Unlock()
	  for _, row := range rows {
		  var h idemHold
		  if err := json.Unmarshal(row.Body, &h); err != nil {
			  return fmt.Errorf("idem hold decode: %w", err)
		  }
		   if h.Key == "" {
			   continue
		  }
		   cp := h
		  x.rows[h.Key] = &cp
	  }
	  return nil
   }

   func (x *idemIndex) save(h idemHold) error {
	  if x == nil || h.Key == "" {
		   return nil
	   }
	   x.mu.Lock()
	   kv := x.kv
	   x.mu.Unlock()
	   if kv == nil {
		  return nil
	   }
	  body, err := json.Marshal(h)
	   if err != nil {
		   return err
	   }
	  return kvPut(context.Background(), kv, kindWDIdem, h.Key, body)
   }

// BindGuardKV 挂守恒、刷提、审计与半冻结引用的同一 KV，并回填。
// 回填失败时新出金失败即关；已在内存的跳闸不会因此去解冻。
   func (s *Service) BindGuardKV(kv persist.KV) error {
	   if s == nil || kv == nil {
		   return nil
	   }
	   if s.conserve != nil {
		   if err := s.conserve.BindKV(kv); err != nil {
			  s.conserve.setStoreErr(err)
			  return err
		   }
	   }
	  if s.abuse != nil {
		   if err := s.abuse.BindKV(kv); err != nil {
			  if s.conserve != nil {
				  s.conserve.setStoreErr(err)
			   }
			  return err
		   }
	   }
	   if s.audits != nil {
		   if err := s.audits.BindKV(kv); err != nil {
			   if s.conserve != nil {
				  s.conserve.setStoreErr(err)
			  }
			  return err
		   }
	  }
	   if s.idemIdx != nil {
		   if err := s.idemIdx.BindKV(kv); err != nil {
			   if s.conserve != nil {
				  s.conserve.setStoreErr(err)
			   }
			   return err
		  }
	   }
	  if s.phases != nil {
		  if err := s.phases.BindKV(kv); err != nil {
			  if s.conserve != nil {
				  s.conserve.setStoreErr(err)
			   }
			  return err
		  }
		  if err := s.hydratePhases(); err != nil {
			   if s.conserve != nil {
				  s.conserve.setStoreErr(err)
			   }
			  return err
		  }
	  }
	  if s.conserve != nil {
		  s.conserve.setStoreErr(nil)
	  }
	   return nil
   }
