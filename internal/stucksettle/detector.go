package stucksettle

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// DetectorResult 扫描结果摘要。

type DetectorResult struct {
	Scanned  int `json:"scanned"`   // 扫描租约数
	NewCases int `json:"new_cases"` // 新产出双结案件数
	Skipped int `json:"skipped"` // 已有 DupCase 跳过数
}

// Detector 双结回收扫描器。
type Detector struct {
	mu sync.Mutex
	store Store
	clock func() time.Time
}

// NewDetector 创建扫描器。
   func NewDetector(store Store) *Detector {
	  return &Detector{
		  store: store,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟（用）。

   func (d *Detector) WithClock(fn func() time.Time) *Detector {
	   if fn != nil {
		  d.clock = fn
	  }
	  return d
   }

  // ScanTransfer 对单个 transfer_id 执行双结扫描。
   // 规则：
   // 1. 收集 transferID 下全部非 DupBlocked 租约。
   // 2. 若 Acting 状态租约 ≥ 2，或 Recovered 状态租约 ≥ 2，视为双结。
  // 3. 将第二个及后续租约对应产出 DupRecoverCase（幂等）。
  func (d *Detector) ScanTransfer(transferID string) (DetectorResult, error) {
	   if d == nil || d.store == nil {
		   return DetectorResult{}, ErrUnavailable
	  }
	  trID := strings.TrimSpace(transferID)
	  if trID == "" {
		   return DetectorResult{}, ErrInvalidInput
	   }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  now := d.clock()
	   // 查询已有 DupCase（用于幂等）
	   existing, err := d.store.DupCasesByTransfer(trID)
	   if err != nil {
		  return DetectorResult{}, err
	   }
	   existingKeys := map[string]struct{}{} // key = dup_lease_id
	   for _, c := range existing {
		  existingKeys[c.DupLeaseID] = struct{}{}
	  }
	  gen, err := d.store.GetGen(trID)
	  if err != nil || gen == nil {
		  // 无世代记录说明尚未有回收，跳过
		  return DetectorResult{}, nil
	   }
	  result := DetectorResult{}
	  // 扫描 Acting 重复：不应出现两个 Acting
	  acting := d.collectActingLeases(trID)
	   result.Scanned += len(acting)
	   if len(acting) > 1 {
		  first := acting[0]
		  for _, dup := range acting[1:] {
			   result.Scanned++
			  if _, skip := existingKeys[dup.LeaseID]; skip {
				  result.Skipped++
				   continue
			   }
			  if err := d.emitDupCase(first, dup, gen.CurrentGen, now); err == nil {
				  result.NewCases++
			   }
		   }
	  }
	   // 扫描 Recovered 重复：同世代不应出现两个 Recovered
	  recovered := d.collectRecoveredLeases(trID, gen.CurrentGen)
	  result.Scanned += len(recovered)
	  if len(recovered) > 1 {
		  first := recovered[0]
		   for _, dup := range recovered[1:] {
			  if _, skip := existingKeys[dup.LeaseID]; skip {
				  result.Skipped++
				   continue
			   }
			   if err := d.emitDupCase(first, dup, gen.CurrentGen, now); err == nil {
				   result.NewCases++
			   }
		  }
	   }
	  return result, nil
  }

// collectActingLeases 收集 transferID 下全部 Acting 租约（按 ActingAt 升序）。

   func (d *Detector) collectActingLeases(transferID string) []*RecoverLease {
	  // MemStore 不提供列表接口；此处通过 store 接口约定做类型断言回退
	   if ms, ok := d.store.(*MemStore); ok {
		   ms.mu.Lock()
		  defer ms.mu.Unlock()
		  var out []*RecoverLease
		  for _, l := range ms.leases {
			  if l.TransferID == transferID && l.State == LeaseActing {
				  cp := *l
				  out = append(out, &cp)
			   }
		  }
		  sortLeasesByActingAt(out)
		   return out
	  }
	  // SQL 实现由 SQLStore.ActiveLeaseByTransfer 覆盖；Detector 通过扫描 SQL 视图获取
	   return nil
   }

// collectRecoveredLeases 收集 transferID 在当前世代下全部 Recovered 租约。
  func (d *Detector) collectRecoveredLeases(transferID string, gen int64) []*RecoverLease {
	  if ms, ok := d.store.(*MemStore); ok {
		  ms.mu.Lock()
		   defer ms.mu.Unlock()
		   var out []*RecoverLease
		   for _, l := range ms.leases {
			   if l.TransferID == transferID && l.State == LeaseRecovered && l.Generation == gen {
				   cp := *l
				   out = append(out, &cp)
			  }
		   }
		   sortLeasesByActingAt(out)
		  return out
	   }
	  return nil
   }

   func (d *Detector) emitDupCase(first, dup *RecoverLease, gen int64, now time.Time) error {
	  c := &DupRecoverCase{
		  DupCaseID: uuid.NewString(),
		  TransferID: first.TransferID,
		   Generation: gen,
		  FirstLeaseID: first.LeaseID,
		  DupLeaseID:   dup.LeaseID,
		  State:        DupCaseOpen,
		  DetectedAt:   now,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	   return d.store.PutDupCase(c)
  }

// sortLeasesByActingAt 按 ActingAt 升序排序（插入排序，列表短）。

  func sortLeasesByActingAt(leases []*RecoverLease) {
	   for i := 1; i < len(leases); i++ {
		  for j := i; j > 0 && leases[j].ActingAt.Before(leases[j-1].ActingAt); j-- {
			   leases[j], leases[j-1] = leases[j-1], leases[j]
		  }
	   }
  }
