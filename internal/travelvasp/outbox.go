package travelvasp

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

const (
	kindTravelOutbox       = "travel.outbox"
	kindTravelOutboxPoison = "travel.outbox.poison"
	outboxMaxTry           = 8
)

// OutboxItem 外部协议案件重试条。
type OutboxItem struct {
	CaseID string `json:"case_id"`
	Proto string `json:"proto"`
	Peer   string    `json:"peer"`
	Tries  int       `json:"tries"`
	NextAt time.Time `json:"next_at"`
	Last string `json:"last,omitempty"`
}

type CaseOutbox struct {
	kv persist.KV
}

   func NewCaseOutbox(kv persist.KV) *CaseOutbox {
	   if kv == nil {
		  kv = persist.NewMemKV()
	   }
	   return &CaseOutbox{kv: kv}
   }

   func (s *Service) BindOutbox(kv persist.KV) *Service {
	  if s != nil && kv != nil {
		  s.outbox = NewCaseOutbox(kv)
	   }
	   return s
  }

  func (s *Service) enqueueOutbox(c Case, proto, detail string) {
	   if s == nil || s.outbox == nil || strings.TrimSpace(c.ID) == "" {
		  return
	  }
	  item := OutboxItem{
		  CaseID: c.ID,
		  Proto:  proto, Peer: c.PeerVASP, Tries: 0,
		   NextAt: time.Now().UTC().Add(2 * time.Second), Last: detail,
	   }
	  body, err := json.Marshal(item)
	  if err != nil {
		  return
	   }
	   _, existed, err := persist.GetOrInit(context.Background(), s.outbox.kv, kindTravelOutbox, c.ID, body)
	  if err != nil {
		   return
	  }
	   if existed {
		   _ = persist.Replace(context.Background(), s.outbox.kv, kindTravelOutbox, c.ID, body)
	   }
  }

  func (s *Service) outboxRetry(id string) int {
	  if s == nil || s.outbox == nil {
		   return 0
	   }
	   row, err := s.outbox.kv.Get(context.Background(), kindTravelOutbox, strings.TrimSpace(id))
	  if err != nil {
		   return 0
	  }
	  var item OutboxItem
	   if json.Unmarshal(row.Body, &item) != nil {
		   return 0
	   }
	   return item.Tries
  }

// TickOutbox 单次出箱节拍。失败退避；毒丸不伪造 ack。
  func (s *Service) TickOutbox(ctx context.Context) error {
	  if s == nil || s.outbox == nil {
		  return nil
	  }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   rows, err := s.outbox.kv.List(ctx, kindTravelOutbox, 100)
	  if err != nil {
		   return err
	   }
	  now := time.Now().UTC()
	  for _, row := range rows {
		   var item OutboxItem
		  if json.Unmarshal(row.Body, &item) != nil {
			   continue
		  }
		   if item.NextAt.After(now) {
			  continue
		  }
		   cs, err := s.GetCase(item.CaseID)
		  if err != nil {
			  continue
		   }
		   if cs.Status != CaseSubmitted {
			  _ = s.outbox.kv.Delete(ctx, kindTravelOutbox, item.CaseID, row.Fence)
			  continue
		  }
		   peer, err := s.GetVASP(item.Peer)
		   if err != nil {
			  item.Tries++
			  item.Last = err.Error()
			  item.NextAt = now.Add(backoffOut(item.Tries))
			   s.saveOutbox(ctx, item, row.Fence)
			  continue
		  }
		   rec, serr := s.submitAdapter(ctx, peer, cs.Envelope)
		   if serr != nil {
			  s.RecordPeer(peer.Code, false)
			  if err := s.consumeRetryBudget(cs.Corridor); err != nil {
				   s.putDeadLetter(ctx, item, cs.Corridor)
				  s.quarantinePoison(ctx, item, row.Fence)
				  continue
			  }
			   item.Tries++
			   item.Last = serr.Error()
			  metrics.FundGates.WithLabelValues("travel", "outbox_retry").Inc()
			   if item.Tries >= outboxMaxTry {
				   metrics.FundGates.WithLabelValues("travel", "outbox_poison").Inc()
				   slo.FireMiddlewareAlertDetail("travel.outbox.poison", float64(item.Tries), float64(outboxMaxTry), item.CaseID)
				  s.quarantinePoison(ctx, item, row.Fence)
				   continue
			  }
			   item.NextAt = now.Add(backoffOut(item.Tries))
			  s.saveOutbox(ctx, item, row.Fence)
			   continue
		  }
		   if rec.Status == "acked" && strings.TrimSpace(rec.VendorID) != "" {
			   s.RecordPeer(peer.Code, true)
			   _, _ = s.AdvanceCase(cs.ID, cs.Actor, CaseAcknowledged, "vendor receipt "+rec.VendorID)
			   if cs.MsgID != "" {
				  _, _ = s.AdvanceOut(cs.MsgID, cs.Actor, OutAcked)
			   }
			  _ = s.outbox.kv.Delete(ctx, kindTravelOutbox, item.CaseID, row.Fence)
			  continue
		  }
		   item.Tries++
		  item.Last = rec.Detail
		  item.NextAt = now.Add(backoffOut(item.Tries))
		   s.saveOutbox(ctx, item, row.Fence)
	   }
	  return nil
  }

  func (s *Service) saveOutbox(ctx context.Context, item OutboxItem, exp persist.Fence) {
	  body, err := json.Marshal(item)
	  if err != nil || s == nil || s.outbox == nil {
		  return
	   }
	  _ = s.outbox.kv.Put(ctx,
		  persist.Row{Kind: kindTravelOutbox, Key: item.CaseID, Body: body}, exp)
  }

   func backoffOut(tries int) time.Duration {
	  if tries < 1 {
		   tries = 1
	  }
	  shift := tries
	   if shift > 6 {
		  shift = 6
	   }
	  d := time.Duration(1<<uint(shift)) * time.Second
	  if d > 2*time.Minute {
		   d = 2 * time.Minute
	  }
	   if d < 2*time.Second {
		   d = 2 * time.Second
	  }
	  return d
  }

   func (s *Service) quarantinePoison(ctx context.Context, item OutboxItem, exp persist.Fence) {
	  if s == nil || s.outbox == nil {
		   return
	  }
	   body, err := json.Marshal(item)
	  if err != nil {
		  return
	  }
	   _, existed, err := persist.GetOrInit(ctx, s.outbox.kv, kindTravelOutboxPoison, item.CaseID, body)
	  if err == nil && existed {
		  _ = persist.Replace(ctx,
			   s.outbox.kv, kindTravelOutboxPoison, item.CaseID, body)
	   }
	  _ = s.outbox.kv.Delete(ctx, kindTravelOutbox, item.CaseID, exp)
  }

// ListPoison 运营列出毒丸出箱。

   func (s *Service) ListPoison(ctx context.Context, limit int) ([]OutboxItem, error) {
	  if s == nil || s.outbox == nil {
		   return []OutboxItem{}, nil
	  }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	   rows, err := s.outbox.kv.List(ctx, kindTravelOutboxPoison, limit)
	   if err != nil {
		  return nil, err
	   }
	  out := make([]OutboxItem, 0, len(rows))
	   for _, row := range rows {
		   var item OutboxItem
		   if json.Unmarshal(row.Body, &item) != nil {
			  continue
		   }
		   out = append(out, item)
	   }
	   return out, nil
  }

// ReplayOutbox 运营重放毒丸或出箱条。tries 归零。
  func (s *Service) ReplayOutbox(ctx context.Context, caseID string) (OutboxItem, error) {
	  if s == nil || s.outbox == nil {
		   return OutboxItem{}, ErrUnavailable
	  }
	  caseID = strings.TrimSpace(caseID)
	   if caseID == "" {
		   return OutboxItem{}, ErrCase
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   item := OutboxItem{CaseID: caseID,
		   Tries: 0, NextAt: time.Now().UTC()}
	  if row, err := s.outbox.kv.Get(ctx, kindTravelOutboxPoison, caseID); err == nil {
		  _ = json.Unmarshal(row.Body, &item)
		  _ = s.outbox.kv.Delete(ctx, kindTravelOutboxPoison, caseID, row.Fence)
	  } else if row, err := s.outbox.kv.Get(ctx, kindTravelOutbox, caseID); err == nil {
		   _ = json.Unmarshal(row.Body, &item)
	  }
	   if cs, err := s.GetCase(caseID); err == nil {
		   item.CaseID = cs.ID
		   item.Peer = cs.PeerVASP
	   } else {
		  return OutboxItem{}, err
	  }
	   item.Tries = 0
	  item.Last = "replay"
	   item.NextAt = time.Now().UTC()
	   body, err := json.Marshal(item)
	  if err != nil {
		   return OutboxItem{}, err
	   }
	   _,
		   existed, err := persist.GetOrInit(ctx, s.outbox.kv, kindTravelOutbox, caseID, body)
	   if err != nil {
		  return OutboxItem{}, err
	   }
	   if existed {
		  if err := persist.Replace(ctx, s.outbox.kv, kindTravelOutbox, caseID, body); err != nil {
			  return OutboxItem{}, err
		  }
	  }
	   return item, nil
  }

// RunOutbox 用 workerloop 周期 Tick 出箱。
  func (s *Service) RunOutbox(ctx context.Context, log zerolog.Logger) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  cfg := workerloop.Config{
		  Name: "travel-outbox", Interval: 5 * time.Second,
		  MaxBackoff: 30 * time.Second, FailThresh: 8, CircuitPause: 15 * time.Second,
	   }
	   return workerloop.Run(ctx, cfg, log, s.TickOutbox)
   }

  func (s *Service) StartOutbox(log zerolog.Logger) {
	   if s == nil || s.outbox == nil {
		  return
	   }
	   go func() {
		  _ = s.RunOutbox(context.Background(),
			   log)
	   }()
   }
