// @FileName: service.go
// @Summary: 围栏旁路引擎服务层——组合 GenerationManager、BypassDetector、ConserveEngine、MutexStore。
// 重点：Fence/Unfence 必须经世代 CAS + 守恒证明双重校验；
// CreditMasterGuard 是 CreditMaster 的旁路挂钩，检测到围栏即创建 BypassCase。
// 风险点：FenceActivate 与 AllocCheck 存在竞态，必须通过世代 CAS 解决；
// UnfenceRequest 在存在未关闭 BypassCase 或守恒漂移时必须拒绝。
// @Author: Jett
// @Created: 2021.03.08

package brokerfence

import (
	"strings"
	"time"
	"github.com/google/uuid"
)

// Service 围栏旁路引擎服务。
type Service struct {
	leases   LeaseStore
	gen      *GenerationManager
	bypass   *BypassDetector
	conserve *ConserveEngine
	tokens *MutexStore
	clock func() time.Time
}

// NewService 构造围栏旁路引擎服务（使用内存持久化，适用于与单机模式）。
  func NewService(reader BookReader) *Service {
	  genStore := NewMemGenerationStore()
	   leaseStore := NewMemLeaseStore()
	  bypassStore := NewMemBypassStore()
	   conserveStore := NewMemConserveStore()
	   return &Service{
		   leases: leaseStore,
		   gen:    NewGenerationManager(genStore),
		   bypass: NewBypassDetector(bypassStore),
		   conserve: NewConserveEngine(conserveStore, reader),
		   tokens: NewMutexStore(0),
		   clock:  func() time.Time { return time.Now().UTC() },
	  }
  }

// NewServiceWith 用外部组件构造服务（模式，注入 SQL 实现）。
  func NewServiceWith(
	   leases LeaseStore,
	   genStore GenerationStore,
	  bypassStore BypassStore,
	  conserveStore ConserveStore,
	  reader BookReader,
	   tokenTTL time.Duration,
  ) *Service {
	  return &Service{
		  leases: leases,
		   gen:    NewGenerationManager(genStore),
		  bypass: NewBypassDetector(bypassStore),
		   conserve: NewConserveEngine(conserveStore, reader),
		   tokens: NewMutexStore(tokenTTL),
		   clock:  func() time.Time { return time.Now().UTC() },
	   }
  }

   func (svc *Service) now() time.Time {
	   if svc.clock != nil {
		   return svc.clock()
	  }
	   return time.Now().UTC()
   }

// ──────────────────────────────────────────────
// 围栏操作
// ──────────────────────────────────────────────
// FenceActivate 激活围栏：
// 1. 读取子账当前世代（expectGen=0 表示首次围栏）；
// 2. CAS 抬升世代（fenced=true）；
// 3. 创建 FenceLease（open→active）；
// 4. 采样主子合计 BeforeTotal；
// 5. 颁发互斥令牌供有权限的运维持有。

  func (svc *Service) FenceActivate(req FenceReq) (*FenceLease, *MutexToken, error) {
	   if err := validateFenceReq(req); err != nil {
		   return nil, nil, err
	   }
	   // 读当前世代。
	   curGen, err := svc.gen.Current(req.SubID)
	   if err != nil {
		  return nil, nil, err
	   }
	  if curGen.Fenced {
		   return nil, nil, ErrLeaseSealConflict
	   }
	   // CAS 抬升世代。
	   newGen,
		   err := svc.gen.BumpFence(req.SubID, curGen.Generation)
	   if err != nil {
		  return nil, nil, err
	   }
	  now := svc.now()
	  lease := &FenceLease{
		  ID:       uuid.NewString(),
		  SubID:    req.SubID,
		   MasterID: req.MasterID,
		  Asset:      req.Asset,
		  Generation: newGen.Generation,
		   State:      LeaseStateOpen,
		   Actor:     req.Actor,
		  Reason:    req.Reason,
		  OpenedAt:  now,
		   UpdatedAt: now,
	  }
	   // open → active。
	   if err := TransitLease(lease, LeaseStateActive, now); err != nil {
		   return nil, nil, err
	   }
	   if err := svc.leases.SaveLease(lease); err != nil {
		   return nil, nil, err
	   }
	   // 采样 BeforeTotal。
	   proof, err := svc.conserve.SampleBefore(lease.ID, req.SubID, req.MasterID, req.Asset)
	  if err != nil {
		   return nil, nil, err
	   }
	  lease.ProofID = proof.ID
	   if err := svc.leases.UpdateLease(lease); err != nil {
		   return nil, nil, err
	   }
	  // 颁发互斥令牌。
	  tok, err := svc.tokens.Issue(lease.ID, req.SubID, req.Actor)
	  if err != nil {
		   return nil, nil, err
	   }
	   return lease, &tok, nil
   }

// UnfenceRequest 申请解围：
// 1. 确认存在 active 的 FenceLease；
// 2. 检测未关闭的 BypassCase（存在则拒绝）；
// 3. 采样 AfterTotal，校验守恒；
// 4. lifting → sealed；
// 5. CAS 抬升世代（fenced=false）；
// 6. 撤销互斥令牌。

   func (svc *Service) UnfenceRequest(req UnfenceReq) (*FenceLease, *ConserveProof, error) {
	  if strings.TrimSpace(req.SubID) == "" || strings.TrimSpace(req.LeaseID) == "" {
		  return nil, nil, ErrInput
	  }
	  lease, err := svc.leases.GetLease(req.LeaseID)
	  if err != nil {
		   return nil, nil, err
	   }
	  if lease.SubID != strings.TrimSpace(req.SubID) {
		   return nil, nil, ErrNotFound
	  }
	   if !LeaseIsWriteBlocking(lease.State) {
		   return nil, nil, ErrLeaseNotActive
	  }
	   // 校验无未关闭旁路案件。
	   hasOpen, err := svc.bypass.HasOpenCases(req.SubID)
	   if err != nil {
		  return nil, nil, err
	   }
	   if hasOpen {
		  return nil, nil, ErrBypassCaseOpen
	   }
	  // 采样 AfterTotal。
	   proof, err := svc.conserve.SampleAfter(lease.ID)
	   if err != nil {
		  return nil, nil, ErrProofMissing
	   }
	  if proof.IsDrifted {
		  return nil, proof, ErrConserveDrift
	   }
	  now := svc.now()
	   // active → lifting → sealed（连续转换）。
	   if lease.State == LeaseStateActive {
		  if err := TransitLease(lease, LeaseStateLifting, now); err != nil {
			  return nil, nil, err
		  }
	  }
	  if err := TransitLease(lease, LeaseStateSealed, now); err != nil {
		   return nil, nil, err
	   }
	   if err := svc.leases.UpdateLease(lease); err != nil {
		  return nil, nil, err
	  }
	  // 读当前世代并 CAS 解围。
	  curGen, err := svc.gen.Current(req.SubID)
	   if err != nil {
		  return nil, nil, err
	   }
	   if _, err := svc.gen.BumpUnfence(req.SubID, curGen.Generation); err != nil {
		   return nil, nil, err
	  }
	   // 撤销互斥令牌。
	   svc.tokens.Revoke(req.SubID)
	   // 清除漂移标记（正常解围无漂移）。
	   svc.conserve.ClearDrift(lease.ID)
	  return lease, proof, nil
  }

// ──────────────────────────────────────────────
// 写路径校验（由 brokerbook 挂钩调用）
// ──────────────────────────────────────────────
// AllocCheck 划拨前校验：世代匹配 + 围栏期令牌验证。
// 通过则返回当前世代；失败 fail-closed。

  func (svc *Service) AllocCheck(req AllocCheckReq) (FenceGeneration, error) {
	  return svc.gen.CheckWriteEligible(req, svc.tokens)
   }

// CreditMasterGuard CreditMaster 旁路挂钩：
// 若子账当前处于围栏态，自动创建 BypassCase（kind=credit_master）并返回 ErrMutexDenied。
// 调用方须将此错误向上传播，禁止静默吞噬。

  func (svc *Service) CreditMasterGuard(subID, masterID, asset, amount string) (*BypassCase, error) {
	   subID = strings.TrimSpace(subID)
	  masterID = strings.TrimSpace(masterID)
	   asset = strings.TrimSpace(asset)
	   amount = strings.TrimSpace(amount)
	  curGen, err := svc.gen.Current(subID)
	  if err != nil || !curGen.Fenced {
		   // 未围栏：直接放行，无需创建案件。
		  return nil, nil
	  }
	  // 找到活跃 lease。
	  lease,
		  err := svc.leases.GetActiveLeaseForSub(subID)
	  if err != nil {
		  // 找不到活跃 lease 时仍挡写，保守原则。
		  return nil, ErrMutexDenied
	  }
	   bc,
		   err := svc.bypass.Detect(lease.ID, subID, masterID, asset, amount, BypassKindCreditMaster)
	   if err != nil {
		  return nil, err
	  }
	  return bc, ErrMutexDenied
  }

// ToSubGuard to_sub 方向旁路挂钩（Fenced 期间 Alloc to_sub 无令牌时调用）。
  func (svc *Service) ToSubGuard(subID, masterID, asset, amount string) (*BypassCase, error) {
	   return svc.detectBypassIfFenced(subID,
		   masterID, asset, amount, BypassKindToSub)
   }

// ToMasterGuard to_master 方向旁路挂钩。
   func (svc *Service) ToMasterGuard(subID, masterID, asset, amount string) (*BypassCase, error) {
	   return svc.detectBypassIfFenced(subID,
		  masterID, asset, amount, BypassKindToMaster)
  }

  func (svc *Service) detectBypassIfFenced(subID, masterID, asset, amount string, kind BypassKind) (*BypassCase, error) {
	   curGen, err := svc.gen.Current(strings.TrimSpace(subID))
	   if err != nil || !curGen.Fenced {
		   return nil, nil
	  }
	  lease, err := svc.leases.GetActiveLeaseForSub(strings.TrimSpace(subID))
	   if err != nil {
		   return nil, ErrMutexDenied
	   }
	   return svc.bypass.Detect(lease.ID,
		   subID, masterID, asset, amount, kind)
   }

// ──────────────────────────────────────────────
// 查询接口
// ──────────────────────────────────────────────
// QueryGeneration 查询子账当前世代与活跃围栏信息。

   func (svc *Service) QueryGeneration(subID string) (GenerationQueryResp, error) {
	   subID = strings.TrimSpace(subID)
	  if subID == "" {
		   return GenerationQueryResp{}, ErrInput
	   }
	   g, err := svc.gen.Current(subID)
	   if err != nil {
		  return GenerationQueryResp{}, err
	   }
	   resp := GenerationQueryResp{FenceGeneration: g}
	  if g.Fenced {
		  lease, err := svc.leases.GetActiveLeaseForSub(subID)
		   if err == nil {
			  resp.ActiveLeaseID = lease.ID
		  }
	  }
	  return resp, nil
   }

// GetLease 按 ID 查询围栏租约。
   func (svc *Service) GetLease(id string) (*FenceLease, error) {
	  return svc.leases.GetLease(strings.TrimSpace(id))
   }

// GetBypassCase 按 ID 查询旁路案件。

   func (svc *Service) GetBypassCase(id string) (*BypassCase, error) {
	  return svc.bypass.Get(strings.TrimSpace(id))
   }

// ListOpenBypassCases 列出子账未关闭旁路案件。
  func (svc *Service) ListOpenBypassCases(subID string) ([]*BypassCase, error) {
	  return svc.bypass.ListOpen(strings.TrimSpace(subID))
  }

// ApproveBypassCase 旁路案件双人确认。

   func (svc *Service) ApproveBypassCase(req BypassConfirmReq) (*BypassCase, error) {
	   bc, err := svc.bypass.Approve(req)
	   if err != nil {
		   return nil, err
	   }
	   // 若案件已关闭且无漂移，尝试清除漂移标记。
	  if bc.State == BypassStateClosed {
		   hasOpen, _ := svc.bypass.HasOpenCases(bc.SubID)
		   if !hasOpen {
			  svc.conserve.ClearDrift(bc.LeaseID)
		  }
	  }
	  return bc, nil
  }

// ExportConserveProof 导出指定 lease 的最新守恒证明。

  func (svc *Service) ExportConserveProof(leaseID string) (*ConserveProof, error) {
	  return svc.conserve.LatestProof(strings.TrimSpace(leaseID))
  }

// IssueToken 运维手动颁发互斥令牌（围栏期间有限写路径入口）。
  func (svc *Service) IssueToken(leaseID, subID, actor string) (MutexToken, error) {
	  return svc.tokens.Issue(leaseID, subID, actor)
   }

// ──────────────────────────────────────────────
// 校验辅助
// ──────────────────────────────────────────────

  func validateFenceReq(req FenceReq) error {
	   if strings.TrimSpace(req.SubID) == "" ||
		   strings.TrimSpace(req.MasterID) == "" ||
		   strings.TrimSpace(req.Asset) == "" ||
		   strings.TrimSpace(req.Actor) == "" {
		   return ErrInput
	  }
	  return nil
  }
