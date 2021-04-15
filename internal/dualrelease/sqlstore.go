// @FileName: sqlstore.go
// @Summary: 双控放行 SQL 仓储适配器。
// SQLStore 将 sqlrepo.DualReleaseRepo 的本进程 Row 类型桥接到 dualrelease.Store 接口。
// 此文件是 dualrelease → sqlrepo 的单向依赖（sqlrepo 不导入 dualrelease，无循环）。
// @Author: Cason
// @Created: 2021.04.14

package dualrelease

import (
	"database/sql"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// SQLStore 包装 sqlrepo.DualReleaseRepo，实现 Store / ReclaimStore / AlignStore 接口。
type SQLStore struct {
	repo *sqlrepo.DualReleaseRepo
}

// NewSQLStore 初始化 SQL Store。
  func NewSQLStore(repo *sqlrepo.DualReleaseRepo) *SQLStore {
	   return &SQLStore{repo: repo}
  }

// ──────────────────────────────────────────────────
// Store 接口实现
// ──────────────────────────────────────────────────

  func (s *SQLStore) PutCase(rc *ReleaseCase) error {
	  return s.repo.PutCase(toCaseRow(rc))
   }

  func (s *SQLStore) GetCase(requestID string) (*ReleaseCase, error) {
	   row, err := s.repo.GetCase(requestID)
	  if err != nil {
		   if err == sqlrepo.ErrNotFound {
			   return nil, ErrNotFound
		  }
		  return nil, err
	  }
	   return fromCaseRow(row), nil
  }

   func (s *SQLStore) PutLease(l *ReleaseLease) error {
	   return s.repo.PutLease(toLeaseRow(l))
   }

   func (s *SQLStore) GetLease(requestID string) (*ReleaseLease, error) {
	   row,
		   err := s.repo.GetLease(requestID)
	   if err != nil {
		   if err == sqlrepo.ErrNotFound {
			   return nil, persist.ErrNotFound
		   }
		   return nil, err
	  }
	  return fromLeaseRow(row), nil
  }

  func (s *SQLStore) PutProof(p *EffectProof) error {
	  return s.repo.PutProof(toProofRow(p))
   }

  func (s *SQLStore) GetProof(requestID string, gen int64) (*EffectProof, error) {
	  row, err := s.repo.GetProof(requestID, gen)
	   if err != nil {
		  if err == sqlrepo.ErrNotFound {
			   return nil, fmt.Errorf("dualrelease: effect proof not found for request=%s gen=%d", requestID, gen)
		  }
		   return nil, err
	   }
	  return fromProofRow(row), nil
  }

   func (s *SQLStore) AppendExpiry(e *TicketExpiry) error {
	  return s.repo.AppendExpiry(toExpiryRow(e))
  }

   func (s *SQLStore) ListExpiredCases(now time.Time) ([]*ReleaseCase, error) {
	   rows, err := s.repo.ListExpiredCases(now)
	  if err != nil {
		   return nil, err
	   }
	   out := make([]*ReleaseCase, len(rows))
	  for i, r := range rows {
		  out[i] = fromCaseRow(r)
	  }
	   return out, nil
  }

// ──────────────────────────────────────────────────

   func (s *SQLStore) ListExecutingCases() ([]*ReleaseCase, error) {
	   rows, err := s.repo.ListExecutingCases()
	  if err != nil {
		  return nil, err
	   }
	   out := make([]*ReleaseCase,
		  len(rows))
	  for i, r := range rows {
		  out[i] = fromCaseRow(r)
	   }
	  return out, nil
  }

// ──────────────────────────────────────────────────
// ProofLister 接口（供 HTTP 导出接口使用）
// ──────────────────────────────────────────────────

  func (s *SQLStore) ListProofs(requestID string) ([]*EffectProof, error) {
	   rows, err := s.repo.ListProofs(requestID)
	  if err != nil {
		   return nil, err
	  }
	  out := make([]*EffectProof, len(rows))
	  for i, r := range rows {
		  out[i] = fromProofRow(r)
	   }
	   return out, nil
  }

// ──────────────────────────────────────────────────
// 接口断言
// ──────────────────────────────────────────────────

var (
	_ Store        = (*SQLStore)(nil)
	_ ReclaimStore = (*SQLStore)(nil)
	_ AlignStore   = (*SQLStore)(nil)
	_ ProofLister  = (*SQLStore)(nil)
)

// ──────────────────────────────────────────────────
// Row 转换函数
// ──────────────────────────────────────────────────

  func toCaseRow(rc *ReleaseCase) sqlrepo.DualReleaseCaseRow {
	   return sqlrepo.DualReleaseCaseRow{
		   ID: rc.ID,
		   RequestID: rc.RequestID,
		  TicketID: rc.TicketID,
		   Gen: rc.Gen,
		  State:         string(rc.State),
		   Actor:         rc.Actor,
		  FundActionID:  rc.FundActionID,
		   EffectProofID: rc.EffectProofID,
		   AbortReason: rc.AbortReason,
		   FenceEpoch: rc.Fence.Epoch,
		   FenceToken: rc.Fence.Token,
		   CreatedAt:  rc.CreatedAt,
		   UpdatedAt:  rc.UpdatedAt,
	  }
  }

   func fromCaseRow(r sqlrepo.DualReleaseCaseRow) *ReleaseCase {
	   return &ReleaseCase{
		   ID: r.ID,
		  RequestID: r.RequestID,
		   TicketID:  r.TicketID,
		  Gen:       r.Gen,
		  State:     ReleaseState(r.State),
		  Actor:     r.Actor,
		   FundActionID:  r.FundActionID,
		   EffectProofID: r.EffectProofID,
		   AbortReason: r.AbortReason,
		   Fence: persist.Fence{Epoch: r.FenceEpoch, Token: r.FenceToken},
		   CreatedAt: r.CreatedAt,
		  UpdatedAt: r.UpdatedAt,
	  }
   }

   func toLeaseRow(l *ReleaseLease) sqlrepo.DualReleaseLeaseRow {
	  row := sqlrepo.DualReleaseLeaseRow{
		   ID:        l.ID,
		   RequestID: l.RequestID,
		  Holder: l.Holder,
		   Gen:        l.Gen,
		  State:      string(l.State),
		   LeaseToken: l.LeaseToken,
		  AcquiredAt: l.AcquiredAt,
		  ExpiresAt: l.ExpiresAt,
		  FenceEpoch: l.Fence.Epoch,
		  FenceToken: l.Fence.Token,
		   CreatedAt:  l.CreatedAt,
		  UpdatedAt:  l.UpdatedAt,
	   }
	  if l.ReleasedAt != nil {
		   row.ReleasedAt = sql.NullTime{Time: *l.ReleasedAt, Valid: true}
	   }
	   return row
   }

   func fromLeaseRow(r sqlrepo.DualReleaseLeaseRow) *ReleaseLease {
	  l := &ReleaseLease{
		   ID: r.ID,
		   RequestID: r.RequestID,
		   Holder:    r.Holder,
		   Gen: r.Gen,
		  State:      ReleaseState(r.State),
		   LeaseToken: r.LeaseToken,
		   AcquiredAt: r.AcquiredAt,
		   ExpiresAt:  r.ExpiresAt,
		   Fence: persist.Fence{Epoch: r.FenceEpoch, Token: r.FenceToken},
		   CreatedAt: r.CreatedAt,
		   UpdatedAt: r.UpdatedAt,
	  }
	   if r.ReleasedAt.Valid {
		   t := r.ReleasedAt.Time
		  l.ReleasedAt = &t
	   }
	  return l
  }

  func toExpiryRow(e *TicketExpiry) sqlrepo.DualTicketExpiryRow {
	  return sqlrepo.DualTicketExpiryRow{
		  ID: e.ID,
		   TicketID:  e.TicketID,
		   RequestID: e.RequestID,
		   Reason:    string(e.Reason),
		   EpochAt:   e.EpochAt,
		   TTLSecs:   e.TTLSecs,
		   ExpiredAt: e.ExpiredAt,
		  Actor: e.Actor,
		  AuditHash: e.AuditHash,
		   CreatedAt: e.CreatedAt,
	  }
   }

   func toProofRow(p *EffectProof) sqlrepo.DualEffectProofRow {
	  return sqlrepo.DualEffectProofRow{
		  ID:           p.ID,
		  RequestID:    p.RequestID,
		  TicketID:     p.TicketID,
		   Gen:          p.Gen,
		  FundActionID: p.FundActionID,
		  Actor: p.Actor,
		   ProofHash:  p.ProofHash,
		   SignedAt:   p.SignedAt,
		  FenceEpoch: p.Fence.Epoch,
		  FenceToken: p.Fence.Token,
		  CreatedAt:  p.CreatedAt,
	  }
  }

  func fromProofRow(r sqlrepo.DualEffectProofRow) *EffectProof {
	   return &EffectProof{
		   ID: r.ID,
		  RequestID: r.RequestID,
		  TicketID:  r.TicketID,
		  Gen:       r.Gen,
		   FundActionID: r.FundActionID,
		  Actor: r.Actor,
		  ProofHash: r.ProofHash,
		  SignedAt:  r.SignedAt,
		  Fence: persist.Fence{Epoch: r.FenceEpoch,
			  Token: r.FenceToken},
		   CreatedAt: r.CreatedAt,
	  }
  }
