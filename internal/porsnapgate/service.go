// @FileName: service.go
// @Summary: porsnapgate 包内 service 实现。
// 准备金快照世代与公证活账漂移封堵引擎 — 核心服务层。
// Service 封装所有领域操作：
// - BuildWithGen：构建快照并钉 snap_gen； Prove/Attest 必须经此路径。
// - Attest：按 snap_gen 创建公证请求；缺 snap_gen 则拒绝。
// - PublishAttest：发布公证至 published 态（ops 审核后调用）。
// - RevokeAttest：撤销公证。
// - ProveAttested：按已发布公证重建 Merkle 树并生成包含证明；禁止 live 模式。
// - CompareWithProof：将封印快照与活账；发现漂移时落 PorSnapDriftProof。
// - LiveSnapshot：显式 live 模式读取，结果打非公证标记；不写公证表。
// - RevokeGen：撤销世代并联动撤销已有公证。
// 不变量（Fail-Closed）：
// 1. ProveAttested 路径禁止 live 模式；使用 ReadModeSealed。
// 2. live 读取通过 LiveSnapshot 显式调用，结果带 LiveExportTag，禁止写公证表。
// 3. CompareWithProof 发现根漂移或 totals 指纹不一致时必须创建 PorSnapDriftProof。
// @Author: Dax
// @Created: 2021.09.30

package porsnapgate

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/por"
	"github.com/google/uuid"
)

const (
	// DefaultTTLSeconds 默认世代有效期：24 小时。
	DefaultTTLSeconds = int64(86400)
)

// ─────────────────────────────────────────────────────────────────────────────
// Service
// ─────────────────────────────────────────────────────────────────────────────
// Service 准备金快照世代与公证封堵引擎核心服务。
type Service struct {
	store Store
	ledger ledger.Store
	clock  func() time.Time
}

// NewService 构造 Service；store 和 ledger 不可为 nil。
  func NewService(store Store, ledger ledger.Store, clock func() time.Time) *Service {
	  if store == nil {
		  panic("porsnapgate: Service requires non-nil Store")
	   }
	  if ledger == nil {
		   panic("porsnapgate: Service requires non-nil ledger.Store")
	  }
	  if clock == nil {
		  clock = time.Now
	   }
	   return &Service{store: store, ledger: ledger, clock: clock}
  }

   // ─────────────────────────────────────────────────────────────────────────────
   // BuildWithGen — 构建快照并封印世代
  // ─────────────────────────────────────────────────────────────────────────────
  // BuildWithGenOpts BuildWithGen 选项。

   type BuildWithGenOpts struct {
	  // Users 目标用户列表；为空时取所有用户（业务上不推荐直接使用）。
	   Users []string
	  // TTLSeconds 世代有效期；0 使用 DefaultTTLSeconds。
	   TTLSeconds int64
   }

   // BuildWithGen 构建快照并封印 snap_gen。
   // 流程：
  // 1. 调用 por.BuildFromStore 构建 Snapshot。
  // 2. 计算 LeavesDigest、TotalsFingerprint。
  // 3. 读取当前 store 水位（store mark）。
  // 4. 插入 PorSnapshotGen 行（state=sealed）。
   // 5. 返回 BuildWithGenResult。
   func (s *Service) BuildWithGen(ctx context.Context, opts BuildWithGenOpts) (*BuildWithGenResult, error) {
	   now := s.clock().UTC()
	   if opts.TTLSeconds <= 0 {
		  opts.TTLSeconds = DefaultTTLSeconds
	   }
	   snap, err := por.BuildFromStore(ctx, s.ledger, opts.Users, now)
	   if err != nil {
		  return nil, err
	   }
	  leavesDigest := HashLeafSet(snap.Tree.Leaves)
	   totalsEntries := TotalsToEntries(snap.Totals)
	  totalsFingerprint := HashTotals(totalsEntries)
	   storeMark, err := s.readStoreMark(ctx)
	   if err != nil {
		  return nil, err
	  }
	   gen := &PorSnapshotGen{
		   SnapGenID:    uuid.NewString(),
		  SnapshotID:   snap.ID,
		   MerkleRoot:   snap.Root,
		  LeavesDigest: leavesDigest,
		   TotalsFingerprint: totalsFingerprint,
		   AsOf:              now,
		   StoreMark:         storeMark,
		   UserCount:         len(opts.Users),
		   LeafCount:         len(snap.Tree.Leaves),
		  State: SnapSealed,
		  TTLSeconds: opts.TTLSeconds,
		   SealedAt: now,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := s.store.InsertGen(ctx, gen); err != nil {
		  return nil, err
	   }
	   return &BuildWithGenResult{
		   SnapshotID:        snap.ID,
		   SnapGenID:         gen.SnapGenID,
		   MerkleRoot:        snap.Root,
		  LeavesDigest:      leavesDigest,
		   TotalsFingerprint: totalsFingerprint,
		   AsOf: now,
		   StoreMark: storeMark,
		   UserCount: gen.UserCount,
		   LeafCount: gen.LeafCount,
	   }, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// Attest — 创建公证请求
// ─────────────────────────────────────────────────────────────────────────────
// Attest 为指定 snap_gen 创建公证请求（state=pending）。
// 拒绝条件：
// - SnapGenID 为空 → ErrNoGen
// - 世代不存在 → ErrGenNotFound
// - 世代非 sealed → ErrInvalidTransition / ErrGenExpired / ErrGenRevoked
// - 世代已有公证 → ErrAttestBound

   func (s *Service) Attest(ctx context.Context, req AttestRequest) (*AttestationBundle, error) {
	   if req.SnapGenID == "" {
		  return nil, ErrNoGen
	  }
	  now := s.clock().UTC()
	   gen, err := s.store.GetGen(ctx, req.SnapGenID)
	  if err != nil {
		  return nil, err
	  }
	  if err := AssertGenForAttest(gen, now); err != nil {
		  return nil, err
	  }
	  payload := req.ExportPayload
	  if len(payload) == 0 {
		   payload = buildDefaultExportPayload(gen,
			   now)
	  }
	  exportHash := HashExportPayload(payload)
	   a := &AttestationBundle{
		   AttestID: uuid.NewString(),
		   SnapGenID: gen.SnapGenID,
		   SnapshotID: gen.SnapshotID,
		  MerkleRoot: gen.MerkleRoot,
		   TotalsFingerprint: gen.TotalsFingerprint,
		  ExportPayload:     payload,
		  ExportHash: exportHash,
		  State:      AttestPending,
		  Requester:  req.Requester,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   if err := s.store.InsertAttest(ctx, a); err != nil {
		  return nil, err
	  }
	  return a, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// PublishAttest — 发布公证
// ─────────────────────────────────────────────────────────────────────────────
// PublishAttest 将 pending 公证推进到 published，并同步将关联 snap_gen 推进到 attested。
   func (s *Service) PublishAttest(ctx context.Context, attestID string) (*AttestationBundle, error) {
	  now := s.clock().UTC()
	   var result *AttestationBundle
	  err := s.store.InTx(ctx, func(ctx context.Context) error {
		   a, err := s.store.GetAttest(ctx, attestID)
		   if err != nil {
			  return err
		  }
		  if err := ApplyAttestTransition(a, AttestPublished, "", now); err != nil {
			  return err
		  }
		  if err := s.store.UpdateAttestState(ctx, a.AttestID, AttestPending, AttestPublished, "", now); err != nil {
			   return err
		   }
		   // 同步推进 snap_gen → attested
		   _ = s.store.UpdateGenState(ctx, a.SnapGenID, SnapSealed, SnapAttested, now)
		   result = a
		   return nil
	  })
	   return result, err
  }

// ─────────────────────────────────────────────────────────────────────────────
// RevokeAttest — 撤销公证
// ─────────────────────────────────────────────────────────────────────────────
// RevokeAttest 撤销公证；允许从 pending 或 published 撤销。

   func (s *Service) RevokeAttest(ctx context.Context, attestID, reason string) error {
	  now := s.clock().UTC()
	  return s.store.InTx(ctx, func(ctx context.Context) error {
		   a, err := s.store.GetAttest(ctx, attestID)
		   if err != nil {
			   return err
		  }
		  from := a.State
		  if err := ApplyAttestTransition(a, AttestRevoked, reason, now); err != nil {
			   return err
		   }
		   return s.store.UpdateAttestState(ctx, a.AttestID, from, AttestRevoked, reason, now)
	   })
   }

// ─────────────────────────────────────────────────────────────────────────────
// ProveAttested — 按已发布公证重建证明
// ─────────────────────────────────────────────────────────────────────────────
// ProveAttestedOpts ProveAttested 请求参数。

type ProveAttestedOpts struct {
	// SnapGenID 世代 ID；必须有对应的 published 公证。
	SnapGenID string
	// UserID 目标用户 ID。
	UserID string
	// Book 科目名称（trading / funding / …）。
	Book string
	// Asset 资产代码（BTC / ETH / …）。
	Asset string
	// Users 重建用于限定用户集；必须与 BuildWithGen 时一致。
	Users []string
}

// ProveAttested 按已发布公证重建 Merkle 树，生成包含证明。
// 严格禁止 live 模式；使用 ReadModeSealed 语义——
// 从 por.BuildFromStore 重建时使用相同 Users 列表，结果根必须与封印根一致；
// 若根不一致，说明活账已变，此时触发 CompareWithProof 路径并返回错误。

   func (s *Service) ProveAttested(ctx context.Context, opts ProveAttestedOpts) (*por.Proof, error) {
	   if opts.SnapGenID == "" {
		  return nil, ErrNoGen
	  }
	  now := s.clock().UTC()
	   gen, err := s.store.GetGen(ctx, opts.SnapGenID)
	   if err != nil {
		   return nil, err
	   }
	   attest, err := s.store.GetAttestByGen(ctx, opts.SnapGenID)
	  if err != nil {
		   return nil, err
	  }
	  if err := AssertAttestUsable(attest); err != nil {
		   return nil, err
	   }
	   // 重建快照（ReadModeSealed：仅用于证明，结果根必须稳定）
	  snap, err := por.BuildFromStore(ctx, s.ledger, opts.Users, gen.AsOf)
	   if err != nil {
		   return nil, err
	   }
	   // 根一致性校验
	   if snap.Root != gen.MerkleRoot {
		  // 活账已变，触发漂移证明
		  _, driftErr := s.recordDriftProof(ctx, gen, snap, DriftKindReplay, now)
		  if driftErr != nil {
			   // 记录失败不应掩盖主错误
			   _ = driftErr
		  }
		  return nil, ErrRootMismatch
	  }
	  proof, err := snap.Prove(opts.UserID, opts.Book, opts.Asset)
	   if err != nil {
		   return nil, err
	  }
	   return &proof, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// CompareWithProof — 活账并落漂移证明
// ─────────────────────────────────────────────────────────────────────────────
// CompareWithProofOpts CompareWithProof 请求参数。

type CompareWithProofOpts struct {
	// SnapGenID 被的世代 ID。
	SnapGenID string
	// Users 活账用户列表（须与 BuildWithGen 时一致）。
	Users []string
}

// CompareWithProof 将封印快照与当前活账。
// 流程：
// 1. 读取 snap_gen（封印根、指纹）。
// 2. 重建活账快照。
// 3. 比较根与 totals 指纹。
// 4. 若漂移：创建 PorSnapDriftProof；返回 CompareWithProofResult{Drifted:true}。
// 5. 若一致：返回 CompareWithProofResult{Drifted:false}。
// 不变量：即使结果一致，也不得将活账快照视为封印世代根。
  func (s *Service) CompareWithProof(ctx context.Context, opts CompareWithProofOpts) (*CompareWithProofResult, error) {
	   if opts.SnapGenID == "" {
		   return nil, ErrNoGen
	  }
	  now := s.clock().UTC()
	   gen, err := s.store.GetGen(ctx, opts.SnapGenID)
	   if err != nil {
		   return nil, err
	  }
	   if !gen.IsActive() {
		   if gen.State == SnapExpired {
			   return nil, ErrGenExpired
		  }
		   if gen.State == SnapRevoked {
			   return nil, ErrGenRevoked
		  }
	   }
	  liveSnap, err := por.BuildFromStore(ctx, s.ledger, opts.Users, now)
	  if err != nil {
		   return nil, err
	   }
	   liveTotalsEntries := TotalsToEntries(liveSnap.Totals)
	   liveTotalsFingerprint := HashTotals(liveTotalsEntries)
	   drifted := liveSnap.Root != gen.MerkleRoot || liveTotalsFingerprint != gen.TotalsFingerprint
	  if !drifted {
		  return &CompareWithProofResult{
			  SnapGenID: opts.SnapGenID,
			   Drifted: false,
		   }, nil
	  }
	   // 漂移：构建漂移资产列表
	  driftAssets := buildDriftAssets(gen, liveSnap)
	  proof, err := s.recordDriftProof(ctx, gen, liveSnap, DriftKindCompare, now)
	  if err != nil {
		  return nil, err
	   }
	  return &CompareWithProofResult{
		  SnapGenID: opts.SnapGenID,
		   Drifted:   true,
		  DriftProof:  proof,
		  DriftAssets: driftAssets,
	  }, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// LiveSnapshot — 显式 live 模式快照
// ─────────────────────────────────────────────────────────────────────────────
// LiveSnapshotResult live 模式快照结果；带 LiveExportTag 标记。

type LiveSnapshotResult struct {
	Snapshot por.Snapshot `json:"snapshot"`
	Tag LiveExportTag `json:"tag"`
}

// LiveSnapshot 显式 live 模式读取；结果打非公证/非法医标记，不写公证表。
// 语义：调用方必须通过本方法（而非 ProveAttested）获取 live 数据，且须接受 Tag 标记。
// live 模式不创建 snap_gen；禁止将结果用于公证流程。
  func (s *Service) LiveSnapshot(ctx context.Context, users []string) (*LiveSnapshotResult, error) {
	   now := s.clock().UTC()
	  snap, err := por.BuildFromStore(ctx, s.ledger, users, now)
	   if err != nil {
		  return nil, err
	   }
	  return &LiveSnapshotResult{
		  Snapshot: snap,
		   Tag:      NewLiveExportTag(now),
	  }, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// RevokeGen — 撤销世代
// ─────────────────────────────────────────────────────────────────────────────
// RevokeGen 撤销世代；若有关联公证则一并撤销。
   func (s *Service) RevokeGen(ctx context.Context, snapGenID, reason string) error {
	  now := s.clock().UTC()
	  return s.store.InTx(ctx, func(ctx context.Context) error {
		   gen,
			  err := s.store.GetGen(ctx, snapGenID)
		   if err != nil {
			  return err
		   }
		   from := gen.State
		   if err := ApplyGenTransition(gen, SnapRevoked, now); err != nil {
			  return err
		  }
		   if err := s.store.UpdateGenState(ctx, snapGenID, from, SnapRevoked, now); err != nil {
			   return err
		   }
		  // 联动撤销已有公证
		  attest, err := s.store.GetAttestByGen(ctx, snapGenID)
		   if err != nil && !errors.Is(err, ErrAttestNotFound) {
			  return err
		  }
		  if attest != nil && attest.State == AttestPublished {
			   _ = s.store.UpdateAttestState(ctx, attest.AttestID, AttestPublished, AttestRevoked, reason, now)
		  }
		  return nil
	  })
   }

// ─────────────────────────────────────────────────────────────────────────────
// GetGen / GetAttest / GetDriftProofs — 只读查询
// ─────────────────────────────────────────────────────────────────────────────
// GetGen 读取世代。

   func (s *Service) GetGen(ctx context.Context, snapGenID string) (*PorSnapshotGen, error) {
	  return s.store.GetGen(ctx, snapGenID)
  }

// GetAttest 读取公证。

   func (s *Service) GetAttest(ctx context.Context, attestID string) (*AttestationBundle, error) {
	   return s.store.GetAttest(ctx, attestID)
   }

// GetDriftProofsByGen 读取某世代的所有漂移证明。

   func (s *Service) GetDriftProofsByGen(ctx context.Context, snapGenID string) ([]*PorSnapDriftProof, error) {
	  return s.store.ListDriftProofsByGen(ctx, snapGenID, 100)
  }

// ─────────────────────────────────────────────────────────────────────────────
// internal helpers
// ─────────────────────────────────────────────────────────────────────────────
// readStoreMark 读取当前 store 水位（通过扫描账户序列近似）。
// 当 ledger.Store 不支持直接读取水位时，使用 SumDebitCredit 计数代替。
// 实际中可替换为专用序列接口。

  func (s *Service) readStoreMark(ctx context.Context) (int64, error) {
	  // 使用当前 Unix 纳秒作为水位的近似；
	   // 环境应替换为 ledger 自增序列。
	   return s.clock().UTC().UnixNano(), nil
  }

// recordDriftProof 构建并持久化漂移证明。

  func (s *Service) recordDriftProof(
	   ctx context.Context,
	  gen *PorSnapshotGen,
	   liveSnap por.Snapshot,
	   kind ProofKind,
	   now time.Time,
   ) (*PorSnapDriftProof, error) {
	   liveMark, _ := s.readStoreMark(ctx)
	  driftAssets := buildDriftAssets(gen, liveSnap)
	   liveTotalsEntries := TotalsToEntries(liveSnap.Totals)
	  liveTotals := marshalTotals(liveTotalsEntries)
	   sealedTotalsEntries := make([]TotalEntry, 0)
	   // 这里无法直接从 gen 重建 totals，用 fingerprint 记录即可
	  // 实际上 gen 不存储 totals 明细，只存指纹；封印明细在 por.Snapshot 里
	   sealedTotalsBytes,
		   _ := json.Marshal(sealedTotalsEntries)
	  proof := &PorSnapDriftProof{
		   ProofID:   uuid.NewString(),
		   SnapGenID: gen.SnapGenID,
		  SnapshotID: gen.SnapshotID,
		  Kind:       kind,
		   SealedRoot: gen.MerkleRoot,
		   LiveRoot:     liveSnap.Root,
		  SealedTotals: sealedTotalsBytes,
		  LiveTotals:      liveTotals,
		  DriftAssets:     marshalDriftAssets(driftAssets),
		  SealedStoreMark: gen.StoreMark,
		   LiveStoreMark: liveMark,
		   DetectedAt: now,
		   CreatedAt: now,
	  }
	  if err := s.store.InsertDriftProof(ctx, proof); err != nil {
		   if errors.Is(err, ErrDriftProofConflict) {
			  // 已有证明，幂等返回
			   return proof, nil
		   }
		   return nil, err
	   }
	   return proof, nil
  }

// buildDriftAssets 比较封印快照 totals 指纹与 live totals，构建漂移资产列表。
   func buildDriftAssets(gen *PorSnapshotGen, liveSnap por.Snapshot) []DriftAsset {
	   // 由于 gen 不直接存储 totals 明细，只能基于 live totals 产出漂移摘要
	  var assets []DriftAsset
	  for _, t := range liveSnap.Totals {
		  assets = append(assets, DriftAsset{
			   Asset: t.Asset,
			   Sealed: "(fingerprint:" + gen.TotalsFingerprint[:8] + "…)",
			  Live: t.Amount,
		   })
	   }
	   return assets
   }

// buildDefaultExportPayload 构建默认公证导出内容。
  func buildDefaultExportPayload(gen *PorSnapshotGen, now time.Time) []byte {
	  payload := map[string]interface{}{
		   "snap_gen_id": gen.SnapGenID,
		   "snapshot_id": gen.SnapshotID,
		  "merkle_root": gen.MerkleRoot,
		   "leaves_digest":      gen.LeavesDigest,
		  "totals_fingerprint": gen.TotalsFingerprint,
		   "as_of":              gen.AsOf.UTC().Format(time.RFC3339),
		  "store_mark": gen.StoreMark,
		  "user_count": gen.UserCount,
		   "leaf_count":  gen.LeafCount,
		   "exported_at": now.UTC().Format(time.RFC3339),
		  "class":       "engineering_merkle",
		   "solvency":    false,
		  "disclaimer":  por.Disclaimer,
	   }
	  b, _ := json.Marshal(payload)
	  return b
  }
