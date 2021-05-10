package feequotelease

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	// DefaultTTLSeconds 默认租约 TTL（300 秒）。
	DefaultTTLSeconds = 300
	// MaxTTLSeconds 最大允许 TTL（1800 秒）。
	MaxTTLSeconds = 1800
	// ExpireBatchSize 过期 Worker 单次扫描数量上限。
	ExpireBatchSize = 200
	// ProofExportBatchSize 证明导出单次批量。
	ProofExportBatchSize = 100
)

// FeeScheduleReader 从 feeeng 只读消费 Schedule 与分档信息的接口。
// 仅读，不回写 feeeng 内核。
type FeeScheduleReader interface {
	// GetSchedule 查询当前生效的 Schedule（按 kind/asset）。
	GetSchedule(ctx context.Context, kind, asset string) (ScheduleSnapshot, error)
}

// ScheduleSnapshot feeeng Schedule 的只读快照（避免循环依赖）。
type ScheduleSnapshot struct {
	ID    string
	Kind  string
	Asset string
	BPS   int
	MinFee string
	MaxFee  string
	Flat    string
	Version int
}

// Service 手续费报价租约核心服务。
type Service struct {
	store  Store
	reader FeeScheduleReader
	clock func() time.Time
}

// New 创建 Service（使用内存存储，用）。

   func New(reader FeeScheduleReader) *Service {
	   return &Service{
		  store: NewInMemStore(),
		  reader: reader,
		  clock: time.Now,
	   }
  }

// NewWithStore 创建 Service（注入持久化存储）。
   func NewWithStore(store Store, reader FeeScheduleReader) *Service {
	   return &Service{store: store,
		   reader: reader, clock: time.Now}
   }

// WithClock 注入时钟（用）。
   func (s *Service) WithClock(fn func() time.Time) *Service {
	  s.clock = fn
	   return s
  }

   // ─────────────────────────────────────────────────────────────────────────────
  // QuoteLease
  // ─────────────────────────────────────────────────────────────────────────────
  // QuoteLease 签发手续费报价租约。
  // 流程：
  // 1. 从 feeeng 读取 ScheduleSnapshot（只读）；
   // 2. 查询当前 sch_gen，若无记录则以 Schedule.Version 初始化；
  // 3. 计算 tiers 指纹（若提供 tiers）；
   // 4. 计算报价费率（仅依赖 BPS，不调用 feeeng Apply）；
   // 5. 签发 FeeQuoteLease 并持久化；
  // 6. 返回租约 ID 与报价结果。
   func (s *Service) QuoteLease(ctx context.Context, req QuoteLeaseReq, tiers []Tier) (FeeQuoteLease, error) {
	  if err := s.validateQuoteReq(req); err != nil {
		   return FeeQuoteLease{}, err
	  }
	   // 幂等检查
	  if req.IdempotencyKey != "" {
		  if existing, ok, err := s.store.GetLeaseByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
			   return FeeQuoteLease{}, fmt.Errorf("%w: idem lookup: %v", ErrUnavailable, err)
		   } else if ok {
			   return existing, nil
		   }
	  }
	  kind := strings.ToLower(strings.TrimSpace(req.Kind))
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   snap, err := s.reader.GetSchedule(ctx, kind, asset)
	   if err != nil {
		  return FeeQuoteLease{}, fmt.Errorf("%w: %v", ErrScheduleNotFound, err)
	  }
	  gen, err := s.currentGen(ctx, kind, asset)
	  if err != nil && err != ErrGenNotFound {
		   return FeeQuoteLease{}, fmt.Errorf("%w: get gen: %v", ErrUnavailable, err)
	   }
	   if err == ErrGenNotFound || gen.Gen == 0 {
		   gen = ScheduleGeneration{
			  ID: uuid.New().String(),
			  Kind: kind,
			   Asset:      asset,
			  Gen:        int64(snap.Version),
			  ScheduleID: snap.ID,
			   BPS: snap.BPS,
			  PutAt: s.clock(),
			  PutBy: "init",
		   }
		  if gen.Gen == 0 {
			  gen.Gen = 1
		  }
		   _ = s.store.SaveGen(ctx, gen)
	   }
	  tiersFingerprint := TiersFingerprint(tiers)
	  bps := snap.BPS
	   if len(tiers) > 0 && strings.TrimSpace(req.Volume) != "" {
		   picked,
			  err := pickTier(req.Volume, tiers)
		   if err == nil {
			  bps = picked.BPS
		  }
	   }
	  quotedFee, quotedNet, err := computeFee(req.Gross, bps)
	   if err != nil {
		  return FeeQuoteLease{}, fmt.Errorf("%w: compute fee: %v", ErrInvalidReq, err)
	   }
	   ttl := req.TTLSeconds
	  if ttl <= 0 {
		  ttl = DefaultTTLSeconds
	  }
	   if ttl > MaxTTLSeconds {
		   ttl = MaxTTLSeconds
	   }
	  now := s.clock()
	  lease := FeeQuoteLease{
		   ID:     uuid.New().String(),
		  Kind:   kind,
		   Asset:  asset,
		  Gross:  req.Gross,
		   Volume: req.Volume,
		   QuotedBPS: bps,
		   QuotedFee:        quotedFee,
		   QuotedNet:        quotedNet,
		   ScheduleID:       snap.ID,
		  SchGen:           gen.Gen,
		   TiersFingerprint: tiersFingerprint,
		  State:          LeaseStateQuoted,
		   ExpiresAt:      now.Add(time.Duration(ttl) * time.Second),
		   QuotedAt:       now,
		   IdempotencyKey: req.IdempotencyKey,
	  }
	  if err := s.store.SaveLease(ctx, lease); err != nil {
		   return FeeQuoteLease{}, fmt.Errorf("%w: save lease: %v", ErrUnavailable, err)
	  }
	   return lease, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// ApplyBound
// ─────────────────────────────────────────────────────────────────────────────
// ApplyBound 消费租约执行扣费，必须复验 sch_gen 与 tiers 指纹。
// Fail-closed：未携带 lease_id 则直接拒绝（落 DriftNoLease 证明）。
// 漂移检测顺序：expired → revoked → consumed → sch_gen → fingerprint。
// 幂等：同一 idempotency_key 重复提交时返回相同 ApplyResult（不双扣）。

  func (s *Service) ApplyBound(ctx context.Context, req ApplyBoundReq, tiers []Tier) (ApplyResult, error) {
	  // Fail-closed
	  if strings.TrimSpace(req.LeaseID) == "" {
		  proof := s.buildProof(DriftNoLease, FeeQuoteLease{}, 0, "", 0, "", "no lease_id provided")
		   _ = s.store.SaveProof(ctx, proof)
		   return ApplyResult{}, ErrNoLease
	   }
	  lease, err := s.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  return ApplyResult{}, err
	   }
	  // 幂等检查：已消费且 idempotency_key 匹配
	   if lease.State == LeaseStateApplied && req.IdempotencyKey != "" && lease.IdempotencyKey == req.IdempotencyKey {
		   return ApplyResult{Lease: lease, Idempotent: true}, nil
	  }
	  now := s.clock()
	  // 状态检查
	   switch lease.State {
	   case LeaseStateExpired:
		  proof := s.buildProof(DriftLeaseExpired, lease, lease.SchGen, lease.TiersFingerprint, lease.QuotedBPS, "lease already expired", "")
		  _ = s.store.SaveProof(ctx, proof)
		  return ApplyResult{}, ErrLeaseExpired
	  case LeaseStateRevoked:
		  proof := s.buildProof(DriftLeaseRevoked, lease, lease.SchGen, lease.TiersFingerprint, lease.QuotedBPS, "lease was revoked by schedule change", "")
		   _ = s.store.SaveProof(ctx, proof)
		   return ApplyResult{}, ErrLeaseRevoked
	  case LeaseStateApplied:
		  proof := s.buildProof(DriftLeaseConsumed, lease, lease.SchGen, lease.TiersFingerprint, lease.QuotedBPS, "lease already applied", "")
		  _ = s.store.SaveProof(ctx, proof)
		  return ApplyResult{}, ErrLeaseConsumed
	   }
	   // TTL 检查
	  if now.After(lease.ExpiresAt) {
		  t := now
		  lease.State = LeaseStateExpired
		   lease.ExpiredAt = &t
		  _ = s.store.SaveLease(ctx, lease)
		  proof := s.buildProof(DriftLeaseExpired, lease, lease.SchGen, lease.TiersFingerprint, lease.QuotedBPS, "ttl exceeded at apply time", "")
		  _ = s.store.SaveProof(ctx, proof)
		  return ApplyResult{}, ErrLeaseExpired
	  }
	  // 查询当前 sch_gen
	   currentGen, err := s.currentGen(ctx, lease.Kind, lease.Asset)
	  if err != nil {
		   return ApplyResult{}, fmt.Errorf("%w: get gen: %v", ErrUnavailable, err)
	  }
	   // sch_gen 复验
	  if currentGen.Gen > lease.SchGen {
		  evidence := fmt.Sprintf(`{"bind_sch_gen":%d,"current_sch_gen":%d,"lease_id":%q}`,
			  lease.SchGen, currentGen.Gen, lease.ID)
		  proof := s.buildProofFull(DriftSchGenAdvanced, lease, currentGen.Gen, currentGen.TiersFingerprint, currentGen.BPS, evidence)
		  _ = s.store.SaveProof(ctx, proof)
		   return ApplyResult{}, fmt.Errorf("%w: bind=%d current=%d", ErrSchGenDrift, lease.SchGen, currentGen.Gen)
	   }
	   // tiers 指纹复验
	  currentFingerprint := TiersFingerprint(tiers)
	  if !FingerprintMatch(lease.TiersFingerprint, currentFingerprint) {
		   evidence := fmt.Sprintf(`{"bind_fp":%q,"current_fp":%q,"lease_id":%q}`,
			   lease.TiersFingerprint,
			  currentFingerprint, lease.ID)
		  proof := s.buildProofFull(DriftFingerprintMismatch, lease, currentGen.Gen, currentFingerprint, currentGen.BPS, evidence)
		  _ = s.store.SaveProof(ctx, proof)
		   return ApplyResult{}, ErrFingerprintMismatch
	   }
	  // 消费租约
	   t := now
	   lease.State = LeaseStateApplied
	  lease.AppliedAt = &t
	  if req.IdempotencyKey != "" {
		  lease.IdempotencyKey = req.IdempotencyKey
	   }
	   if err := s.store.SaveLease(ctx, lease); err != nil {
		   return ApplyResult{}, fmt.Errorf("%w: save lease: %v", ErrUnavailable, err)
	   }
	   return ApplyResult{Lease: lease, Idempotent: false}, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// PutSchedule
// ─────────────────────────────────────────────────────────────────────────────
// PutSchedule 更新档位表，抬升 sch_gen，并吊销全部未消费旧租约。
// 此方法是 feeeng.Book.Put 的上层封装（只读消费 feeeng，不回写其内核）。
// 世代抬升后，所有 kind/asset 的 quoted 租约将被标记为 revoked，
// 使后续 ApplyBound 必然失败并落 FeeQuoteDriftProof。
   func (s *Service) PutSchedule(ctx context.Context, req SchedulePutReq) (ScheduleGeneration, int, error) {
	  if err := s.validatePutReq(req); err != nil {
		   return ScheduleGeneration{}, 0, err
	  }
	   kind := strings.ToLower(strings.TrimSpace(req.Kind))
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   if asset == "" {
		   asset = "*"
	  }
	   // 查询当前 gen，准备抬升
	   prevGen, _ := s.currentGen(ctx, kind, asset)
	  snap, err := s.reader.GetSchedule(ctx, kind, asset)
	  if err != nil {
		  return ScheduleGeneration{}, 0, fmt.Errorf("%w: %v", ErrScheduleNotFound, err)
	   }
	   newGenVal := prevGen.Gen + 1
	  if newGenVal <= 0 {
		  newGenVal = 1
	   }
	   // 如果 snap.Version 更高则以它为准
	   if int64(snap.Version) > newGenVal {
		  newGenVal = int64(snap.Version)
	  }
	   newGen := ScheduleGeneration{
		  ID:         uuid.New().String(),
		   Kind:       kind,
		  Asset:      asset,
		  Gen:        newGenVal,
		  ScheduleID: snap.ID,
		   BPS: req.BPS,
		  PutAt: s.clock(),
		  PutBy: req.PutBy,
	   }
	  if err := s.store.SaveGen(ctx, newGen); err != nil {
		   return ScheduleGeneration{}, 0, fmt.Errorf("%w: save gen: %v", ErrUnavailable, err)
	   }
	  // 吊销未消费旧租约
	  revokedCount, err := s.revokeActiveLeases(ctx, kind, asset,
		   fmt.Sprintf("schedule_put|kind=%s|asset=%s|new_sch_gen=%d", kind, asset, newGenVal))
	  if err != nil {
		   return newGen, revokedCount, fmt.Errorf("revoke leases: %w", err)
	  }
	  return newGen, revokedCount, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// Worker 接口
// ─────────────────────────────────────────────────────────────────────────────
// ExpireLeases 批量过期超 TTL 的 quoted 租约（由 Worker 周期调用）。
// 返回本次过期租约数量。

  func (s *Service) ExpireLeases(ctx context.Context) (int, error) {
	  now := s.clock()
	   leases, err := s.store.ListExpiredLeases(ctx, now, ExpireBatchSize)
	  if err != nil {
		   return 0, fmt.Errorf("%w: list expired: %v", ErrUnavailable, err)
	   }
	   count := 0
	  for _, l := range leases {
		  t := now
		   l.State = LeaseStateExpired
		  l.ExpiredAt = &t
		   if err := s.store.SaveLease(ctx, l); err != nil {
			  continue
		   }
		   count++
	  }
	  return count, nil
   }

// RevokeByKindAsset 吊销指定 kind/asset 的全部 quoted 租约（Schedule 变更时调用）。
   func (s *Service) RevokeByKindAsset(ctx context.Context, kind, asset, reason string) (int, error) {
	  return s.revokeActiveLeases(ctx, kind, asset, reason)
   }

// ExportProofs 批量导出未导出的漂移事故证明。
  func (s *Service) ExportProofs(ctx context.Context) ([]FeeQuoteDriftProof, error) {
	  proofs,
		  err := s.store.ListProofs(ctx, ProofExportBatchSize)
	   if err != nil {
		   return nil, fmt.Errorf("%w: list proofs: %v", ErrUnavailable, err)
	   }
	   now := s.clock()
	  for _, p := range proofs {
		  _ = s.store.MarkProofExported(ctx, p.ID, now)
	  }
	  return proofs, nil
  }

// GetLease 查询单条租约（Ops 调用）。
  func (s *Service) GetLease(ctx context.Context, id string) (FeeQuoteLease, error) {
	   return s.store.GetLease(ctx, id)
  }

// ─────────────────────────────────────────────────────────────────────────────
// 内部辅助
// ─────────────────────────────────────────────────────────────────────────────

   func (s *Service) currentGen(ctx context.Context, kind, asset string) (ScheduleGeneration, error) {
	  g, err := s.store.GetCurrentGen(ctx, kind, asset)
	  if err == ErrGenNotFound {
		   // 尝试通配符
		  g, err = s.store.GetCurrentGen(ctx, kind, "*")
	  }
	  return g, err
   }

  func (s *Service) revokeActiveLeases(ctx context.Context, kind, asset, reason string) (int, error) {
	   leases, err := s.store.ListActiveLeasesByKindAsset(ctx, kind, asset)
	  if err != nil {
		  return 0, fmt.Errorf("%w: list active: %v", ErrUnavailable, err)
	  }
	  // 也吊销通配符租约（若 asset 非 *）
	  var wildcardLeases []FeeQuoteLease
	   if asset != "*" {
		  wildcardLeases, _ = s.store.ListActiveLeasesByKindAsset(ctx, kind, "*")
	  }
	   all := append(leases, wildcardLeases...)
	  now := s.clock()
	   count := 0
	   for _, l := range all {
		   t := now
		   l.State = LeaseStateRevoked
		   l.RevokedAt = &t
		   l.RevokeReason = reason
		   if err := s.store.SaveLease(ctx, l); err != nil {
			  continue
		   }
		   count++
	   }
	   return count, nil
   }

  func (s *Service) buildProof(kind DriftProofKind, lease FeeQuoteLease, currentSchGen int64, currentFingerprint string, currentBPS int, reason, extra string) FeeQuoteDriftProof {
	  ev := buildEvidence(map[string]interface{}{
		  "reason":   reason,
		  "extra":    extra,
		  "lease_id": lease.ID,
		   "fee_kind": lease.Kind,
		  "asset": lease.Asset,
		   "bind_sch_gen":    lease.SchGen,
		   "current_sch_gen": currentSchGen,
	   })
	   return FeeQuoteDriftProof{
		   ID:   uuid.New().String(),
		  Kind: kind,
		  LeaseID: lease.ID,
		   FeeKind: lease.Kind,
		  Asset:      lease.Asset,
		   Gross:      lease.Gross,
		   BindSchGen: lease.SchGen,
		  CurrentSchGen: currentSchGen,
		  BindFingerprint:    lease.TiersFingerprint,
		  CurrentFingerprint: currentFingerprint,
		   BindBPS:            lease.QuotedBPS,
		  CurrentBPS: currentBPS,
		  Evidence:   ev,
		  DetectedAt: s.clock(),
	  }
  }

   func (s *Service) buildProofFull(kind DriftProofKind, lease FeeQuoteLease, currentSchGen int64, currentFingerprint string, currentBPS int, evidenceJSON string) FeeQuoteDriftProof {
	  return FeeQuoteDriftProof{
		   ID:   uuid.New().String(),
		   Kind: kind,
		   LeaseID: lease.ID,
		   FeeKind: lease.Kind,
		   Asset:      lease.Asset,
		  Gross:      lease.Gross,
		  BindSchGen: lease.SchGen,
		   CurrentSchGen:   currentSchGen,
		  BindFingerprint: lease.TiersFingerprint,
		   CurrentFingerprint: currentFingerprint,
		   BindBPS:    lease.QuotedBPS,
		   CurrentBPS: currentBPS,
		   Evidence:   evidenceJSON,
		  DetectedAt: s.clock(),
	  }
  }

  func buildEvidence(fields map[string]interface{}) string {
	  b, err := json.Marshal(fields)
	   if err != nil {
		  return fmt.Sprintf(`{"error":%q}`, err.Error())
	   }
	   if len(b) > 4096 {
		   return string(b[:4093]) + "..."
	   }
	   return string(b)
  }

   func (s *Service) validateQuoteReq(req QuoteLeaseReq) error {
	   if strings.TrimSpace(req.Kind) == "" {
		   return fmt.Errorf("%w: kind required", ErrInvalidReq)
	  }
	   if strings.TrimSpace(req.Asset) == "" {
		   return fmt.Errorf("%w: asset required", ErrInvalidReq)
	   }
	  if strings.TrimSpace(req.Gross) == "" {
		   return fmt.Errorf("%w: gross required", ErrInvalidReq)
	   }
	  return nil
  }

   func (s *Service) validatePutReq(req SchedulePutReq) error {
	  if strings.TrimSpace(req.Kind) == "" {
		  return fmt.Errorf("%w: kind required", ErrInvalidReq)
	  }
	  if req.BPS < 0 || req.BPS > 10000 {
		   return fmt.Errorf("%w: bps must be 0-10000", ErrInvalidReq)
	   }
	  return nil
   }

// computeFee 简单 BPS 费率计算（不依赖 feeeng 内核）。
// 返回 fee, net（字符串形式）。
  func computeFee(gross string, bps int) (fee string, net string, err error) {
	  // 使用字符串浮点运算避免循环依赖；中由 money 包提供精确计算
	   f, ok := parseDecimal(gross)
	  if !ok {
		  return "", "", fmt.Errorf("invalid gross: %s", gross)
	   }
	  if bps < 0 || bps > 10000 {
		   return "", "", fmt.Errorf("invalid bps: %d", bps)
	  }
	   feeVal := f * float64(bps) / 10000.0
	  netVal := f - feeVal
	   return formatDecimal(feeVal), formatDecimal(netVal), nil
  }

   func parseDecimal(s string) (float64, bool) {
	   s = strings.TrimSpace(s)
	   if s == "" {
		  return 0, false
	  }
	   var f float64
	   _, err := fmt.Sscanf(s, "%f", &f)
	  return f, err == nil && f >= 0
   }

   func formatDecimal(f float64) string {
	   return fmt.Sprintf("%.8f", f)
  }

// pickTier 按 volume 选档（镜像 feeeng.PickTier，避免循环依赖）。
   func pickTier(volume string, tiers []Tier) (Tier, error) {
	  if len(tiers) == 0 {
		   return Tier{}, fmt.Errorf("no tiers")
	   }
	  vol, ok := parseDecimal(volume)
	  if !ok {
		   return Tier{}, fmt.Errorf("invalid volume: %s", volume)
	   }
	   picked := tiers[0]
	  for _, t := range tiers {
		  th, ok := parseDecimal(t.Notional)
		   if !ok {
			  continue
		   }
		   if vol >= th {
			  picked = t
		  }
	  }
	   return picked, nil
   }
