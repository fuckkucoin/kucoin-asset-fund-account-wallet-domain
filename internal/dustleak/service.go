package dustleak

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Service 持有内存态的泄漏元数据；写路径依赖 Store 落盘（可选）。
// 在无持久化 Store 时以纯内存运行，适合与单机部署。
type Service struct {
	mu sync.Mutex
	// 策略世代历史 [asset] → sorted by Gen
	gens map[string][]*PolicyGen
	// 牌价快照 [snapKey] → *QuoteSnapshot
	snapshots map[string]*QuoteSnapshot
	// 泄漏案例 [leakID] → *LeakCase
	leaks map[string]*LeakCase
	// 按 convertID 索引，防重复 LeakCase
	leaksByConvert map[string]string // convertID → leakID
	// 回滚腿 [rollbackID] → *LeakRollback
	rollbacks map[string]*LeakRollback
	// 挡兑闸 [userID+"|"+asset] → *ConvertGate
	gates map[string]*ConvertGate
	// 引用 dustconv.Service 做只读查询（牌价、策略）
	dust *dustconv.Service
	clock func() time.Time
	store Store // 可选持久化；nil 则纯内存
}

// Store 定义持久化接口；由调用方注入 SQL 实现。
// 所有写操作幂等；读方法在 not-found 时返回 ErrNotFound。

type Store interface {
	SavePolicyGen(g *PolicyGen) error
	LatestPolicyGen(asset string) (*PolicyGen, error)
	SaveSnapshot(q *QuoteSnapshot) error
	SnapshotByKey(snapKey string) (*QuoteSnapshot, error)
	SaveLeakCase(lc *LeakCase) error
	LeakCaseByID(id string) (*LeakCase, error)
	LeakCaseByConvert(convertID string) (*LeakCase, error)
	OpenLeaksByUser(userID,
		asset string) ([]*LeakCase, error)
	SaveRollback(r *LeakRollback) error
	RollbackByLeak(leakID string) (*LeakRollback, error)
	SaveGate(g *ConvertGate) error
	GateByUserAsset(userID, asset string) (*ConvertGate, error)
}

// New 创建 Service 实例。
// dust 必须非 nil；store 可为 nil（纯内存模式）。

   func New(dust *dustconv.Service, store Store) *Service {
	   return &Service{
		  gens:      map[string][]*PolicyGen{},
		  snapshots: map[string]*QuoteSnapshot{},
		   leaks:     map[string]*LeakCase{},
		  leaksByConvert: map[string]string{},
		  rollbacks:      map[string]*LeakRollback{},
		  gates:          map[string]*ConvertGate{},
		   dust:           dust,
		   clock:          func() time.Time { return time.Now().UTC() },
		  store: store,
	  }
   }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	  return s.clock()
  }

// ── PolicyGen ────────────────────────────────────────────────────────────────
// RecordPolicyGen 记录一次策略世代变更（由 dustconv.PutPolicyGen 后调用）。
// 幂等：相同 asset+gen 不重复插入。

   func (s *Service) RecordPolicyGen(asset string, gen uint64, floor, minConvert string, enabled bool, changedBy string) (*PolicyGen, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return nil, ErrAsset
	   }
	   actor := strings.TrimSpace(changedBy)
	  if actor == "" {
		   return nil, ErrActorInvalid
	   }
	   s.mu.Lock()
	   list := s.gens[as]
	   for _, g := range list {
		   if g.Gen == gen {
			  s.mu.Unlock()
			  return g, nil // 幂等
		  }
	   }
	   pg := &PolicyGen{
		   ID:    uuid.NewString(),
		   Asset: as,
		  Gen:   gen,
		   Floor: floor,
		   MinConvert: minConvert,
		  Enabled:    enabled,
		   ChangedBy: actor,
		  CreatedAt: s.now(),
	   }
	  s.gens[as] = append(list, pg)
	   s.mu.Unlock()
	   if s.store != nil {
		  _ = s.store.SavePolicyGen(pg)
	  }
	   return pg, nil
   }

// LatestPolicyGen 返回 asset 当前最新世代（Gen 最大值）。
  func (s *Service) LatestPolicyGen(asset string) (*PolicyGen, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	   list := s.gens[asset]
	  var best *PolicyGen
	  for _, g := range list {
		  if best == nil || g.Gen > best.Gen {
			  best = g
		   }
	   }
	   s.mu.Unlock()
	  if best == nil {
		  if s.store != nil {
			  return s.store.LatestPolicyGen(asset)
		   }
		   return nil, ErrNotFound
	   }
	   cp := best.Copy()
	   return &cp, nil
  }

// ── QuoteSnapshot ────────────────────────────────────────────────────────────
// SnapKey 构造牌价快照的幂等键。

  func SnapKey(userID, asset, idemKey string) string {
	   return userID + "|" + asset + "|" + idemKey
  }

// TakeSnapshot 为一次 Convert 请求记录牌价快照。
// 幂等：相同 snapKey 直接返回已存在快照。
// 调用方须在发起 dustconv.Convert 前调用，将快照 ID 随 Convert 一同传递。
  func (s *Service) TakeSnapshot(userID, asset, idemKey, rateUSDT string, capBps int, policyGen uint64) (*QuoteSnapshot, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return nil, ErrAsset
	  }
	   key := SnapKey(userID, as, idemKey)
	  s.mu.Lock()
	   if existing, ok := s.snapshots[key]; ok {
		   s.mu.Unlock()
		  cp := existing.Copy()
		   return &cp, nil
	  }
	  s.mu.Unlock()
	  snap,
		   err := dustconv.NewQuoteSnapshot(uuid.NewString(), as, rateUSDT, capBps, policyGen)
	  if err != nil {
		   return nil, ErrSnapshot
	  }
	  persist := &QuoteSnapshot{
		  ID: snap.ID,
		  SnapKey: key,
		   Asset:       as,
		  RateUSDT:    snap.RateUSDT,
		  PolicyGen:   policyGen,
		  DriftCapBps: snap.DriftCapBps,
		   SnapAt: snap.SnapAt,
	  }
	   s.mu.Lock()
	  // 二次检查防止并发写入
	  if existing, ok := s.snapshots[key]; ok {
		  s.mu.Unlock()
		  cp := existing.Copy()
		   return &cp, nil
	   }
	   s.snapshots[key] = persist
	  s.mu.Unlock()
	   if s.store != nil {
		  _ = s.store.SaveSnapshot(persist)
	  }
	   return persist, nil
  }

// BindConvert 将 QuoteSnapshot 与已落账的 Convert ID 绑定（Convert 成功后回填）。
  func (s *Service) BindConvert(snapKey, convertID string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   s.mu.Lock()
	   snap, ok := s.snapshots[snapKey]
	  if !ok {
		  s.mu.Unlock()
		   return ErrNotFound
	   }
	   snap.ConvertID = convertID
	  snap.UsedAt = s.now()
	  s.mu.Unlock()
	  if s.store != nil {
		  _ = s.store.SaveSnapshot(snap)
	   }
	   return nil
  }

// ── Floor Guard CAS ──────────────────────────────────────────────────────────
// CheckGenAtConvert 在 Convert 执行临界区校验世代未变。
// snapGen 是 Convert 请求方在进入临界区前快照的世代号。
// currentGen 是本次 Convert 执行时 dustconv.Service 的当前世代。
// 规则：
// - currentGen == snapGen：世代未变，兑出上限与快照 floor 一致，允许接着。
// - currentGen > snapGen：策略已改变；必须重新评估——若当前 floor 更严（更大），
// 本次 Convert 须以当前更严的 floor 重新验证金额；若当前 floor 更松（更小），
// 本次 Convert 必须终止（不允许按更松的 floor 兑出，避免超额）。
// 返回 (floor_to_use, nil) 表示校验通过；返回 ("", err) 表示 Convert 须终止。
  func (s *Service) CheckGenAtConvert(asset string, snapGen uint64, snapFloor string, currentGenF func() (dustconv.PolicyGen, error)) (string, error) {
	  if s == nil {
		  return "", ErrUnavailable
	  }
	   cur, err := currentGenF()
	   if err != nil {
		  return "", ErrPolicy
	   }
	   if cur.Gen == snapGen {
		  // 世代未变
		  return snapFloor, nil
	   }
	   // 世代已变：比较 floor 严格度
	   snapF, err2 := money.Parse(snapFloor)
	   if err2 != nil {
		   return "", ErrAmount
	   }
	   curF,
		  err3 := money.Parse(cur.Floor)
	  if err3 != nil {
		   return "", ErrAmount
	   }
	  if curF.Cmp(snapF) > 0 {
		  // 当前 floor 更大（更严）：使用当前更严的 floor，Convert 仍可进行但更严格
		   return cur.Floor, nil
	  }
	   // 当前 floor 更小（更松）：策略放松了，为防止超额兑出必须终止
	   return "", fmt.Errorf("%w: snap_gen=%d cur_gen=%d snap_floor=%s cur_floor=%s",
		ErrGenMismatch, snapGen, cur.Gen, snapFloor, cur.Floor)
}

// ── Drift Cap Guard ──────────────────────────────────────────────────────────
// CheckDriftAtConvert 校验 Convert 执行时实时牌价是否相对快照上漂超帽。
// 上漂超帽：Convert 必须 fail-closed（禁止用过期高价兑出超额 USDT）。
// 下漂：安全，不限制（下漂只会减少兑出量，不造成超额）。
  func (s *Service) CheckDriftAtConvert(snap *QuoteSnapshot, liveRateUSDT string) error {
	   if s == nil || snap == nil {
		  return ErrUnavailable
	   }
	  ds, err := dustconv.NewQuoteSnapshot(snap.ID, snap.Asset, snap.RateUSDT, snap.DriftCapBps, snap.PolicyGen)
	   if err != nil {
		  return ErrSnapshot
	   }
	  if err := ds.CheckDrift(liveRateUSDT); err != nil {
		   return fmt.Errorf("%w: %v", ErrDriftExceed, err)
	   }
	   return nil
  }

// ── Leak Detection ───────────────────────────────────────────────────────────
// OpenLeakCase 为一条已落账的违规兑换单创建 LeakCase。
// 幂等：同 convertID 不重复打开（返回已有 case）。
   func (s *Service) OpenLeakCase(convertID,
	  userID, asset string, kind LeakKind,
	  actualAmount, actualUSDT, floorAmount, maxUSDT string,
	   leakAmount, leakUSDT string,
	  snapID, policyGenID, detectedBy string,
   ) (*LeakCase, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return nil, ErrUser
	  }
	  as, err := validate.Asset(asset)
	   if err != nil {
		  return nil, ErrAsset
	  }
	  convertID = strings.TrimSpace(convertID)
	   if convertID == "" {
		  return nil, ErrAmount
	   }
	   s.mu.Lock()
	  if existID, ok := s.leaksByConvert[convertID]; ok {
		  lc := s.leaks[existID]
		  s.mu.Unlock()
		  if lc != nil {
			  cp := lc.Copy()
			   return &cp, nil
		   }
	   }
	  now := s.now()
	  lc := &LeakCase{
		   ID:     uuid.NewString(),
		   Kind:   kind,
		  Status: LeakStatusDetected,
		  UserID: uid,
		  Asset: as,
		   ConvertID:    convertID,
		   SnapshotID:   snapID,
		  PolicyGenID:  policyGenID,
		  ActualAmount: actualAmount,
		  ActualUSDT:   actualUSDT,
		   FloorAmount: floorAmount,
		  MaxUSDT:     maxUSDT,
		  LeakAmount:  leakAmount,
		  LeakUSDT:    leakUSDT,
		  DetectedBy: detectedBy,
		  DetectedAt: now,
		   UpdatedAt:  now,
	  }
	  s.leaks[lc.ID] = lc
	  s.leaksByConvert[convertID] = lc.ID
	  s.mu.Unlock()
	  if s.store != nil {
		  _ = s.store.SaveLeakCase(lc)
	  }
	  // 打开挡兑闸
	   _, _ = s.blockGate(uid, as, lc.ID, "detector")
	  return lc, nil
  }

// GetLeakCase 按 ID 获取 LeakCase。

   func (s *Service) GetLeakCase(id string) (*LeakCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	   lc := s.leaks[id]
	   s.mu.Unlock()
	  if lc != nil {
		   cp := lc.Copy()
		  return &cp, nil
	  }
	  if s.store != nil {
		  return s.store.LeakCaseByID(id)
	  }
	   return nil, ErrNotFound
  }

// ListOpenLeaks 返回指定用户+资产的所有开放泄漏案例。

  func (s *Service) ListOpenLeaks(userID, asset string) ([]*LeakCase, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   s.mu.Lock()
	   var out []*LeakCase
	   for _, lc := range s.leaks {
		  if lc.UserID == userID && (asset == "" || lc.Asset == asset) &&
			   (lc.Status == LeakStatusDetected || lc.Status == LeakStatusRollingBack) {
			   cp := lc.Copy()
			   out = append(out, &cp)
		   }
	  }
	  s.mu.Unlock()
	   return out, nil
   }

// TransitionLeak 执行 LeakCase 状态跳转。
   func (s *Service) TransitionLeak(id string, from, to LeakStatus, actor string) (*LeakCase, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  actor = strings.TrimSpace(actor)
	   if actor == "" {
		  return nil, ErrActorInvalid
	  }
	   s.mu.Lock()
	  lc := s.leaks[id]
	  if lc == nil {
		  s.mu.Unlock()
		   return nil, ErrNotFound
	  }
	   if lc.Status != from {
		  cur := lc.Copy()
		   s.mu.Unlock()
		  return &cur, fmt.Errorf("%w: got=%s want=%s", ErrState, lc.Status, from)
	  }
	   lc.Status = to
	  lc.UpdatedAt = s.now()
	  if to == LeakStatusClosed {
		  lc.ClosedBy = actor
	  }
	  cp := lc.Copy()
	   s.mu.Unlock()
	  if s.store != nil {
		  _ = s.store.SaveLeakCase(lc)
	  }
	   if to == LeakStatusClosed {
		   _ = s.releaseGate(lc.UserID, lc.Asset, actor)
	   }
	  return &cp, nil
   }

// ── Rollback ─────────────────────────────────────────────────────────────────
// PostRollback 为 LeakCase 创建回滚腿并置 RollingBack。
// 幂等：同 leakID 不重复创建。
// 创建后调用方须执行实际账务回冲（逆记 USDT），再调用 ConfirmRollback 确认。
  func (s *Service) PostRollback(leakID, executedBy, dualReqID string) (*LeakRollback, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   if strings.TrimSpace(dualReqID) == "" {
		   return nil, ErrDual
	   }
	  s.mu.Lock()
	  lc := s.leaks[leakID]
	  if lc == nil {
		   s.mu.Unlock()
		   return nil, ErrNotFound
	  }
	   if lc.Status != LeakStatusDetected {
		   cp := lc.Copy()
		  s.mu.Unlock()
		  return nil, fmt.Errorf("%w: cannot post rollback for leak in status=%s", ErrState, cp.Status)
	  }
	   // 检查是否已有回滚腿
	   for _, r := range s.rollbacks {
		  if r.LeakID == leakID {
			   cp := r.Copy()
			  s.mu.Unlock()
			   return &cp, nil
		  }
	  }
	   now := s.now()
	   rb := &LeakRollback{
		  ID: uuid.NewString(),
		   LeakID: leakID,
		  Status: RollbackStatusPending,
		   UserID: lc.UserID,
		   Asset: lc.Asset,
		  ConvertID: lc.ConvertID,
		  RollbackUSDT: lc.LeakUSDT,
		  RollbackAsset: lc.LeakAmount,
		   ExecutedBy:    strings.TrimSpace(executedBy),
		   DualReqID:     dualReqID,
		   CreatedAt:     now,
		  UpdatedAt:     now,
	   }
	   // 计算证明哈希
	   rb.ProofHash = proofHash(rb.ID, rb.ConvertID, rb.RollbackUSDT, rb.RollbackAsset)
	   s.rollbacks[rb.ID] = rb
	  // 推进 LeakCase 状态
	   lc.Status = LeakStatusRollingBack
	   lc.UpdatedAt = now
	  s.mu.Unlock()
	  if s.store != nil {
		   _ = s.store.SaveRollback(rb)
		   _ = s.store.SaveLeakCase(lc)
	  }
	  return rb, nil
   }

// ConfirmRollback 将回滚腿标记为已执行（账务逆记完成）并验证 Conserve。
// proofHashIn 必须与 LeakRollback.ProofHash 一致（由调用方回传）。
   func (s *Service) ConfirmRollback(rollbackID, proofHashIn, actor string) (*LeakRollback, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		  return nil, ErrActorInvalid
	  }
	  s.mu.Lock()
	  rb := s.rollbacks[rollbackID]
	   if rb == nil {
		  s.mu.Unlock()
		   return nil, ErrNotFound
	   }
	   if rb.ProofHash != strings.TrimSpace(proofHashIn) {
		  s.mu.Unlock()
		   return nil, ErrProof
	  }
	   if rb.Status != RollbackStatusPending && rb.Status != RollbackStatusPosted {
		  cp := rb.Copy()
		  s.mu.Unlock()
		  return &cp, fmt.Errorf("%w: status=%s", ErrState, rb.Status)
	  }
	   now := s.now()
	  rb.Status = RollbackStatusConserved
	  rb.PostedAt = now
	   rb.UpdatedAt = now
	   leakID := rb.LeakID
	  s.mu.Unlock()
	   if s.store != nil {
		  _ = s.store.SaveRollback(rb)
	  }
	  // 收口 LeakCase
	  _, _ = s.TransitionLeak(leakID, LeakStatusRollingBack, LeakStatusClosed, actor)
	  cp := rb.Copy()
	   return &cp, nil
   }

// GetRollback 按 ID 获取 LeakRollback。
   func (s *Service) GetRollback(rollbackID string) (*LeakRollback, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	  rb := s.rollbacks[rollbackID]
	   s.mu.Unlock()
	  if rb != nil {
		  cp := rb.Copy()
		   return &cp, nil
	  }
	   if s.store != nil {
		   return s.store.RollbackByLeak(rollbackID)
	   }
	  return nil, ErrNotFound
   }

// RollbackProofOf 生成可外部校验的回滚证明。
  func (s *Service) RollbackProofOf(rollbackID string) (*RollbackProof, error) {
	   rb, err := s.GetRollback(rollbackID)
	  if err != nil {
		   return nil, err
	   }
	  return &RollbackProof{
		  RollbackID: rb.ID,
		   LeakID:       rb.LeakID,
		   ConvertID:    rb.ConvertID,
		  UserID:       rb.UserID,
		   Asset:        rb.Asset,
		   RollbackUSDT: rb.RollbackUSDT,
		  ProofHash: rb.ProofHash,
		  Status:   string(rb.Status),
		  PostedAt: rb.PostedAt,
	  }, nil
   }

// ── ConvertGate ──────────────────────────────────────────────────────────────

   func gateKey(userID, asset string) string { return userID + "|" + asset }

// blockGate 打开挡兑闸（blocked）；若已存在且状态 blocked，幂等返回。

  func (s *Service) blockGate(userID, asset, leakID, blockedBy string) (*ConvertGate, error) {
	   key := gateKey(userID, asset)
	   s.mu.Lock()
	  existing := s.gates[key]
	   if existing != nil && existing.Status == GateStatusBlocked {
		  cp := existing.Copy()
		  s.mu.Unlock()
		   return &cp, nil
	   }
	  now := s.now()
	   gate := &ConvertGate{
		   ID: uuid.NewString(),
		  UserID: userID,
		   Asset:  asset,
		  Status:    GateStatusBlocked,
		  LeakID:    leakID,
		   BlockedBy: blockedBy,
		   BlockedAt: now,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  s.gates[key] = gate
	  s.mu.Unlock()
	   if s.store != nil {
		  _ = s.store.SaveGate(gate)
	   }
	   return gate, nil
   }

// releaseGate 解除挡兑闸；LeakCase 收口时调用。

  func (s *Service) releaseGate(userID, asset, releasedBy string) error {
	   key := gateKey(userID, asset)
	   s.mu.Lock()
	  gate := s.gates[key]
	   if gate == nil {
		   s.mu.Unlock()
		   return nil
	   }
	  now := s.now()
	   gate.Status = GateStatusReleased
	  gate.ReleasedBy = releasedBy
	   gate.ReleasedAt = now
	  gate.UpdatedAt = now
	  s.mu.Unlock()
	   if s.store != nil {
		  _ = s.store.SaveGate(gate)
	  }
	   return nil
  }

// GateStatus 返回用户+资产当前挡兑状态。

  func (s *Service) GateStatusOf(userID, asset string) GateStatus {
	  if s == nil {
		   return GateStatusOpen
	   }
	  s.mu.Lock()
	  gate := s.gates[gateKey(userID,
		   asset)]
	   s.mu.Unlock()
	   if gate != nil && gate.Status == GateStatusBlocked {
		   return GateStatusBlocked
	  }
	  return GateStatusOpen
  }

// ForceBlock 运营双人强制挡兑。
   func (s *Service) ForceBlock(userID, asset, leakID, actor, dualReqID string) (*ConvertGate, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  if strings.TrimSpace(dualReqID) == "" {
		   return nil, ErrDual
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return nil, ErrUser
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return nil, ErrAsset
	  }
	   return s.blockGate(uid, as, strings.TrimSpace(leakID), strings.TrimSpace(actor))
  }

// ForceRelease 运营双人强制解挡（LeakCase 必须已 closed）。
  func (s *Service) ForceRelease(userID, asset, actor, dualReqID string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	  if strings.TrimSpace(dualReqID) == "" {
		  return ErrDual
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return ErrUser
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return ErrAsset
	   }
	  return s.releaseGate(uid, as, strings.TrimSpace(actor))
   }

// ── 辅助 ─────────────────────────────────────────────────────────────────────
// proofHash 计算 SHA-256(rollbackID|convertID|rollbackUSDT|rollbackAsset)。
  func proofHash(rollbackID, convertID, rollbackUSDT, rollbackAsset string) string {
	   h := sha256.New()
	   h.Write([]byte(rollbackID + "|" + convertID + "|" + rollbackUSDT + "|" + rollbackAsset))
	   return fmt.Sprintf("%x", h.Sum(nil))
  }

// leakUSDT 计算 actual - allowed 的超额 USDT。
// 若 actual <= allowed，返回 "0"（无泄漏）。

   func leakUSDT(actualUSDT, maxUSDT string) (string, bool) {
	   actual, err := money.Parse(actualUSDT)
	   if err != nil {
		  return "0", false
	   }
	   max, err := money.Parse(maxUSDT)
	  if err != nil {
		   return "0", false
	  }
	   if actual.Cmp(max) <= 0 {
		  return "0", false
	  }
	   diff := actual.Dec().Sub(max.Dec())
	  if diff.LessThanOrEqual(decimal.Zero) {
		   return "0", false
	  }
	  return diff.String(), true
  }
