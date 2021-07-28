// Service 偏离管线主服务：编排扫描、分级、封堵建议发出与证明落仓。
// 并发模型：
// - 单个 Service 可并发处理多个只读查询请求。
// - Scan 持有 scanMu 互斥锁，防止同一时段同时发起两次扫描。
// - 写操作（Ack、ForceReview）加 mu 保护内存状态并持久化到 Store。
// 封堵语义：
// - crit 批次触发 SealAdvice；本服务只落库建议，不修改账本冻结或广播 FSM。
// - 调用方通过监听事件或轮询 /invdrift/advices/active 决定是否执行挡写。

// Author: Bronson
// Date: 2021.07.27

package invdrift

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// Service invdrift 主服务

type Service struct {
	mu     sync.RWMutex
	scanMu sync.Mutex // 防止并发扫描
	store Store
	ledStore ledger.Store
	custSvc  *custody.Service
	invBook  *hotinv.Book
	thresh   ThresholdConfig
	dc       string // 机房标识
	ttlSec int // SealAdvice 默认 TTL 秒数
	clock func() time.Time
	log   zerolog.Logger
}

// Option Service 配置项

type Option func(*Service)

   func WithDataCenter(dc string) Option {
	  return func(s *Service) { s.dc = dc }
  }

   func WithThresholds(t ThresholdConfig) Option {
	   return func(s *Service) { s.thresh = t }
  }

   func WithTTL(sec int) Option {
	  return func(s *Service) { s.ttlSec = sec }
   }

   func WithClock(fn func() time.Time) Option {
	   return func(s *Service) { s.clock = fn }
}

   func WithLogger(l zerolog.Logger) Option {
	   return func(s *Service) { s.log = l }
   }

// New 构造 Service

   func New(store Store, ledStore ledger.Store, cust *custody.Service, inv *hotinv.Book, opts ...Option) *Service {
	   s := &Service{
		   store: store,
		  ledStore: ledStore,
		   custSvc: cust,
		   invBook: inv,
		   thresh:  DefaultThresholds,
		   ttlSec:  1800,
		  clock:   func() time.Time { return time.Now().UTC() },
	   }
	  for _, o := range opts {
		  o(s)
	  }
	  return s
   }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
  }

// Scan 执行一次完整的偏离扫描并持久化结果。
// 同一时段不允许并发 Scan（返回 ErrConflict）。
  func (s *Service) Scan(ctx context.Context, actor string) (DriftBatch, error) {
	  if s == nil || s.store == nil {
		   return DriftBatch{}, ErrUnavailable
	  }
	   if !s.scanMu.TryLock() {
		  return DriftBatch{}, ErrConflict
	   }
	  defer s.scanMu.Unlock()
	   batchID := uuid.NewString()
	  // 创建 open 批次
	   openBatch := DriftBatch{
		   ID: batchID,
		  State:      BatchStateOpen,
		   TriggerBy:  actor,
		   DataCenter: s.dc,
		  ScanAt:    s.now(),
		   CreatedAt: s.now(),
		   UpdatedAt: s.now(),
	   }
	   if err := s.store.InsertBatch(ctx, openBatch); err != nil {
		   return DriftBatch{}, err
	   }
	   // 标记 scanning
	  openBatch.State = BatchStateScanning
	   openBatch.UpdatedAt = s.now()
	   _ = s.store.UpdateBatch(ctx,
		   openBatch)
	  // 读取三侧数据
	   ledRows,
		  err := LoadLedgerRows(ctx, s.ledStore)
	   if err != nil {
		  return openBatch, err
	   }
	   var custRows []custody.InventoryRow
	  if s.custSvc != nil {
		  custRows, err = s.custSvc.Inventory(ctx)
		   if err != nil {
			   return openBatch, err
		   }
	   }
	  var invSnaps []hotinv.Snapshot
	   if s.invBook != nil {
		   invSnaps,
			   err = s.invBook.Observe(ctx, s.ledStore)
		  if err != nil {
			   return openBatch, err
		  }
	  }
	  // 执行扫描计算
	   out := RunScan(batchID, ScanInput{
		  LedgerRows: ledRows,
		  CustRows:   custRows,
		  InvSnaps:   invSnaps,
		  DataCenter: s.dc,
		   ThreshCfg: s.thresh,
	   }, s.clock)
	  // 持久化条目
	   for i := range out.Items {
		   out.Items[i].BatchID = batchID
	  }
	   if err := s.store.InsertItems(ctx, out.Items); err != nil {
		   return openBatch, err
	   }
	  // 构建最终批次
	   batch := BuildBatchFromItems(batchID, s.dc, actor, out.Items, s.clock)
	  // 生成 SealAdvice
	   critItems := FilterCrit(out.Items)
	   if len(critItems) > 0 {
		   advices := BuildSealAdvices(batchID, critItems, s.ttlSec, s.clock)
		  if err := s.store.InsertAdvices(ctx, advices); err != nil {
			  return batch, err
		  }
		   s.log.Warn().Str("batch_id", batchID).Int("crit", len(critItems)).
			   Msg("invdrift: crit drift detected, seal advices proposed")
	  }
	   // 生成 DriftProof（每个资产一条）
	   for _, bundle := range out.Bundles {
		   var item DriftItem
		  for _, it := range out.Items {
			   if it.Asset == bundle.Asset {
				  item = it
				   break
			  }
		  }
		   proof, err := BuildProof(batchID, bundle.Asset,
			   bundle.Ledger, bundle.Custody, bundle.HotInv,
			  item.DeltaHot, item.DeltaCold, item.Grade)
		   if err != nil {
			   s.log.Error().Err(err).Str("asset", bundle.Asset).Msg("invdrift: build proof failed")
			   continue
		  }
		  proof.ID = uuid.NewString()
		   if err := s.store.InsertProof(ctx, proof); err != nil {
			   s.log.Error().Err(err).Str("asset", bundle.Asset).Msg("invdrift: insert proof failed")
		   }
	   }
	   // 持久化最终批次状态
	  if err := s.store.UpdateBatch(ctx, batch); err != nil {
		  return batch, err
	  }
	   return batch, nil
  }

// GetBatch 查询单个批次

  func (s *Service) GetBatch(ctx context.Context, id string) (DriftBatch, error) {
	   if s == nil || s.store == nil {
		  return DriftBatch{}, ErrUnavailable
	   }
	  return s.store.GetBatch(ctx, id)
  }

// ListBatches 列出批次（分页）
   func (s *Service) ListBatches(ctx context.Context, limit, offset int) ([]DriftBatch, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	   }
	  return s.store.ListBatches(ctx,
		  limit, offset)
  }

// ListItems 列出批次内的偏离条目
  func (s *Service) ListItems(ctx context.Context, batchID string) ([]DriftItem, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	  return s.store.ListItemsByBatch(ctx, batchID)
  }

// ListItemsByAsset 查询某资产的历史偏离条目

   func (s *Service) ListItemsByAsset(ctx context.Context, asset string, limit int) ([]DriftItem, error) {
	  if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	   }
	  a, err := ValidateAsset(asset)
	   if err != nil {
		   return nil, err
	   }
	  return s.store.ListItemsByAsset(ctx, a, limit)
  }

// ListActiveAdvices 查询当前有效的封堵建议

   func (s *Service) ListActiveAdvices(ctx context.Context) ([]SealAdvice, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	   return s.store.ListActiveAdvices(ctx)
  }

// ListAdvicesByBatch 查询批次的封堵建议

   func (s *Service) ListAdvicesByBatch(ctx context.Context, batchID string) ([]SealAdvice, error) {
	  if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	  }
	   return s.store.ListAdvicesByBatch(ctx, batchID)
   }

// AckAdvice 运维确认封堵建议（已处置）

   func (s *Service) AckAdvice(ctx context.Context, req AckRequest) error {
	  if s == nil || s.store == nil {
		   return ErrUnavailable
	  }
	   actor, err := ValidateActor(req.Actor)
	  if err != nil {
		   return err
	  }
	   adv,
		  err := s.store.GetAdvice(ctx, req.AdviceID)
	  if err != nil {
		   return err
	  }
	  if adv.State != AdviceStateProposed {
		  return ErrBadState
	   }
	   now := s.now()
	   return s.store.UpdateAdviceState(ctx, req.AdviceID, AdviceStateAcked,
		  actor, &now, nil, "")
  }

// ForceReview 运维强制复核批次（批次标记 resolved，建议全部 expired）
   func (s *Service) ForceReview(ctx context.Context, req ForceReviewRequest) (DriftBatch, error) {
	   if s == nil || s.store == nil {
		   return DriftBatch{}, ErrUnavailable
	  }
	   _, err := ValidateActor(req.Actor)
	  if err != nil {
		   return DriftBatch{}, err
	   }
	  batch, err := s.store.GetBatch(ctx, req.BatchID)
	   if err != nil {
		   return DriftBatch{}, err
	   }
	  now := s.now()
	   // 所有 proposed 建议标为 expired
	  advices, _ := s.store.ListAdvicesByBatch(ctx, req.BatchID)
	   for _, a := range advices {
		   if a.State == AdviceStateProposed {
			  _ = s.store.UpdateAdviceState(ctx, a.ID, AdviceStateExpired,
				   "", nil, &now, "force_review:"+req.Note)
		   }
	   }
	  // 所有 item 标为 resolved
	   items, _ := s.store.ListItemsByBatch(ctx, req.BatchID)
	  for _, it := range items {
		   if it.ResolvedAt == nil {
			  _ = s.store.MarkItemResolved(ctx, it.ID, now)
		  }
	  }
	  // 批次标为 resolved
	  batch.State = BatchStateResolved
	   batch.ResolvedAt = &now
	   batch.UpdatedAt = now
	  if err := s.store.UpdateBatch(ctx, batch); err != nil {
		  return batch, err
	   }
	  return batch, nil
  }

// GetProof 查询偏离证明
   func (s *Service) GetProof(ctx context.Context, id string) (DriftProof, error) {
	   if s == nil || s.store == nil {
		  return DriftProof{}, ErrUnavailable
	   }
	  return s.store.GetProof(ctx, id)
   }

// ListProofsByBatch 查询批次的偏离证明

  func (s *Service) ListProofsByBatch(ctx context.Context, batchID string) ([]DriftProof, error) {
	  if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	   }
	  return s.store.ListProofsByBatch(ctx,
		   batchID)
   }

// AssetDimension 查询资产维度汇总视图（最近 N 批次）

  func (s *Service) AssetDimension(ctx context.Context, limit int) ([]AssetDimView, error) {
	   if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	   }
	   batches,
		  err := s.store.ListBatches(ctx, limit, 0)
	   if err != nil {
		   return nil, err
	   }
	  // 按资产聚合
	  type entry struct {
		   maxGrade   DriftGrade
		  batchCount int
		   lastScanAt time.Time
		   deltaHot  string
		   deltaCold string
	  }
	   agg := map[string]*entry{}
	  for _, b := range batches {
		   items, err := s.store.ListItemsByBatch(ctx, b.ID)
		   if err != nil {
			   continue
		  }
		  for _, it := range items {
			   e := agg[it.Asset]
			   if e == nil {
				   e = &entry{}
				   agg[it.Asset] = e
			   }
			   e.batchCount++
			   if b.ScanAt.After(e.lastScanAt) {
				   e.lastScanAt = b.ScanAt
				   e.deltaHot = it.DeltaHot
				   e.deltaCold = it.DeltaCold
			  }
			  if gradeSeverity(it.Grade) > gradeSeverity(e.maxGrade) {
				   e.maxGrade = it.Grade
			   }
		   }
	  }
	  out := make([]AssetDimView, 0, len(agg))
	  for asset, e := range agg {
		  out = append(out, AssetDimView{
			   Asset:      asset,
			  MaxGrade:   e.maxGrade,
			  BatchCount: e.batchCount,
			  LastScanAt: e.lastScanAt,
			  DeltaHot:  e.deltaHot,
			   DeltaCold: e.deltaCold,
		   })
	   }
	  return out, nil
   }

// HasActiveAdvice 实现 invcheck.DriftChecker 接口：查询指定资产是否有 proposed 封堵建议。

   func (s *Service) HasActiveAdvice(ctx context.Context, asset string) (bool, error) {
	   if s == nil || s.store == nil {
		  return false, nil
	   }
	  actives,
		   err := s.store.ListActiveAdvices(ctx)
	  if err != nil {
		   return false, err
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  for _, a := range actives {
		   if strings.ToUpper(a.Asset) == asset {
			   return true, nil
		  }
	   }
	   return false, nil
  }

// ExpireStaleAdvices 由 worker 调用：将超过 TTL 的 proposed 建议标为 expired。
   func (s *Service) ExpireStaleAdvices(ctx context.Context) (int, error) {
	   if s == nil || s.store == nil {
		   return 0, ErrUnavailable
	  }
	   actives, err := s.store.ListActiveAdvices(ctx)
	   if err != nil {
		  return 0, err
	  }
	   now := s.now()
	  n := 0
	  for _, a := range actives {
		   deadline := a.ProposedAt.Add(time.Duration(a.TTLSeconds) * time.Second)
		   if now.After(deadline) {
			   if err := s.store.UpdateAdviceState(ctx, a.ID, AdviceStateExpired,
				   "", nil, &now, "ttl_expired"); err == nil {
				  n++
			  }
		  }
	   }
	   return n, nil
   }
