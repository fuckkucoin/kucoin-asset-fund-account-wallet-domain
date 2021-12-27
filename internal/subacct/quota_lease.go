package subacct

import (
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BeginQuotaLease 同事务占用额度并开 pending 租赁（供 Hop saga）。
func (s *Service) BeginQuotaLease(ctx context.Context, parentID, asset, amount string) (string, error) {
	if s == nil || !s.sqlOn() {
		if err := s.reserveQuota(parentID, asset, amount); err != nil {
			return "", err
		}
		return "mem:" + parentID + "|" + strings.ToUpper(asset), nil
	}
	q := s.quotas()
	q.mu.Lock()
	key := quotaKey(parentID, asset)
	row := q.rows[key]
	daily, per := "", ""
	if row != nil {
		daily, per = row.DailyLimit, row.PerTxLimit
	} else if loaded, ok := s.loadQuotaSQL(parentID, asset); ok {
		cp := loaded
		q.rows[key] = &cp
		daily, per = cp.DailyLimit, cp.PerTxLimit
	}
	q.mu.Unlock()
	lease,
		err := sqlrepo.NewSubAcct(s.sql).BeginQuotaLease(ctx, parentID, asset, amount, daily, per, 5*time.Minute)
	if err != nil {
		if errors.Is(err, sqlrepo.ErrConflict) {
			return "", fmt.Errorf("%w: %v", ErrAmount, err)
		}
		return "", err
	}
	return lease.LeaseID, nil
}

// CommitQuotaLease Hop 成功提交租赁。

func (s *Service) CommitQuotaLease(ctx context.Context, leaseID string) error {
	if s == nil || strings.TrimSpace(leaseID) == "" {
		return nil
	}
	if strings.HasPrefix(leaseID, "mem:") {
		return nil
	}
	if !s.sqlOn() {
		return nil
	}
	return sqlrepo.NewSubAcct(s.sql).CommitQuotaLease(ctx, leaseID)
}

// ReleaseQuotaLease Hop 失败释放租赁与额度。

func (s *Service) ReleaseQuotaLease(ctx context.Context, leaseID, parentID, asset, amount string) {
	if s == nil {
		return
	}
	if strings.HasPrefix(leaseID, "mem:") || !s.sqlOn() || strings.TrimSpace(leaseID) == "" {
		s.releaseQuota(parentID, asset, amount)
		return
	}
	if err := sqlrepo.NewSubAcct(s.sql).ReleaseQuotaLease(ctx, leaseID); err != nil {
		s.releaseQuota(parentID, asset, amount)
	}
}

// FlexTransferAtomic 额度租赁 + Hop 更近同事务/saga。
// 先 BeginQuotaLease，再 Hop，成功 Commit，失败 Release。
// 账本/overlay 仍跨库；租赁保证额度侧可回收。

func (s *Service) FlexTransferAtomic(ctx context.Context, typ, parentID, subID, asset, amount, idem string) (HopRec, error) {
	typ = strings.ToUpper(strings.TrimSpace(typ))
	parentID = strings.TrimSpace(parentID)
	subID = strings.TrimSpace(subID)
	var from, to string
	switch typ {
	case "PARENT_TO_SUB":
		from, to = parentID, subID
	case "SUB_TO_PARENT":
		from, to = subID, parentID
	default:
		return HopRec{}, ErrHop
	}
	if err := s.GateFlexType(parentID, subID, typ); err != nil {
		return HopRec{}, err
	}
	leaseID, err := s.BeginQuotaLease(ctx, parentID, asset, amount)
	if err != nil {
		return HopRec{}, err
	}
	got, err := s.Hop(ctx, HopReq{
		FromOwner: from, ToOwner: to, Asset: asset, Amount: amount, IdempotencyKey: idem,
	})
	if err != nil {
		s.ReleaseQuotaLease(ctx, leaseID, parentID, asset, amount)
		return HopRec{}, err
	}
	if cerr := s.CommitQuotaLease(ctx, leaseID); cerr != nil {
		// Hop 已成但租赁未提交；额度已占用，记日志侧由调用方观测；不回滚 Hop。
		_ = cerr
	}
	return got, nil
}

// ExpireStaleQuotaLeases 回收过期额度租赁。

func (s *Service) ExpireStaleQuotaLeases(ctx context.Context, limit int) (int, error) {
	if s == nil || !s.sqlOn() {
		return 0, nil
	}
	return sqlrepo.NewSubAcct(s.sql).ExpireStaleQuotaLeases(ctx, limit)
}
