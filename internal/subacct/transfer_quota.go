package subacct

import (
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/pkg/money"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TransferQuota 母子划转日额度与单笔上限。
// 按母户+资产累计；日切 UTC；配额耗尽拒绝划转；有 SQL 时持久化 used_today。
// 配额只约束 overlay hop，不替代账本余额检查；多实例必须以 SQL 为准。

type TransferQuota struct {
	ParentID   string `json:"parent_id"`
	Asset      string `json:"asset"`
	DailyLimit string `json:"daily_limit"`
	PerTxLimit string `json:"per_tx_limit"`
	UsedToday  string `json:"used_today"`
	Day        string `json:"day"`
}

type quotaBook struct {
	mu   sync.Mutex
	rows map[string]*TransferQuota
}

func (s *Service) quotas() *quotaBook {
	if s == nil {
		return &quotaBook{rows: map[string]*TransferQuota{}}
	}
	if s.quota == nil {
		s.quota = &quotaBook{rows: map[string]*TransferQuota{}}
	}
	return s.quota
}

func quotaKey(parent, asset string) string {
	return strings.ToUpper(strings.TrimSpace(parent)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
}

func (s *Service) persistQuotaSQL(row TransferQuota) error {
	if s == nil || !s.sqlOn() {
		return nil
	}
	return sqlrepo.NewSubAcct(s.sql).PutTransferQuota(context.Background(), sqlrepo.SubTransferQuota{
		ParentID: row.ParentID,
		Asset:    row.Asset, DailyLimit: row.DailyLimit, PerTxLimit: row.PerTxLimit,
		UsedToday: row.UsedToday, Day: row.Day, UpdatedAt: time.Now().UTC(),
	})
}

func (s *Service) loadQuotaSQL(parentID, asset string) (TransferQuota, bool) {
	if s == nil || !s.sqlOn() {
		return TransferQuota{}, false
	}
	got, err := sqlrepo.NewSubAcct(s.sql).GetTransferQuota(context.Background(), parentID, asset)
	if err != nil {
		return TransferQuota{}, false
	}
	return TransferQuota{
		ParentID: got.ParentID, Asset: got.Asset, DailyLimit: got.DailyLimit, PerTxLimit: got.PerTxLimit,
		UsedToday: got.UsedToday, Day: got.Day,
	}, true
}

// HydrateQuotas 从 SQL 装回全部母子划转额度到内存表。
// Open/多实例启动后调用；日切字段以 SQL day 为准。
// 仅 KV/内存 hydrate 不够，缺本步会导致额度重置或漂移。

func (s *Service) HydrateQuotas(ctx context.Context) error {
	if s == nil || !s.sqlOn() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := sqlrepo.NewSubAcct(s.sql).ListTransferQuotas(ctx, "")
	if err != nil {
		return err
	}
	q := s.quotas()
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, r := range rows {
		key := quotaKey(r.ParentID,
			r.Asset)
		q.rows[key] = &TransferQuota{
			ParentID: r.ParentID, Asset: r.Asset, DailyLimit: r.DailyLimit, PerTxLimit: r.PerTxLimit,
			UsedToday: r.UsedToday, Day: r.Day,
		}
	}
	return nil
}

// SetTransferQuota 配置母户某资产日额度与单笔上限。
func (s *Service) SetTransferQuota(parentID, asset, daily, perTx string) (TransferQuota, error) {
	parentID = strings.TrimSpace(parentID)
	asset = strings.ToUpper(strings.TrimSpace(asset))
	if parentID == "" || asset == "" {
		return TransferQuota{}, ErrParent
	}
	if _, err := money.Parse(daily); err != nil {
		return TransferQuota{},
			ErrAmount
	}
	if _, err := money.Parse(perTx); err != nil {
		return TransferQuota{}, ErrAmount
	}
	if err := s.refuseQuotaRaise(parentID, asset, daily, perTx); err != nil {
		return TransferQuota{}, err
	}
	q := s.quotas()
	q.mu.Lock()
	key := quotaKey(parentID, asset)
	row := q.rows[key]
	if row == nil {
		if loaded, ok := s.loadQuotaSQL(parentID, asset); ok {
			cp := loaded
			row = &cp
			q.rows[key] = row
		} else {
			row = &TransferQuota{ParentID: parentID, Asset: asset, UsedToday: "0", Day: time.Now().UTC().Format("2006-01-02")}
			q.rows[key] = row
		}
	}
	s.rollQuotaDayLocked(row)
	row.DailyLimit = daily
	row.PerTxLimit = perTx
	out := *row
	q.mu.Unlock()
	if err := s.persistQuotaSQL(out); err != nil {
		return TransferQuota{}, err
	}
	return out, nil
}

// GetTransferQuota 查询母户划转额度余量。

func (s *Service) GetTransferQuota(parentID, asset string) (TransferQuota, error) {
	parentID = strings.TrimSpace(parentID)
	asset = strings.ToUpper(strings.TrimSpace(asset))
	q := s.quotas()
	q.mu.Lock()
	defer q.mu.Unlock()
	key := quotaKey(parentID, asset)
	row := q.rows[key]
	if row == nil {
		if loaded, ok := s.loadQuotaSQL(parentID, asset); ok {
			cp := loaded
			row = &cp
			q.rows[key] = row
		} else {
			return TransferQuota{ParentID: parentID, Asset: asset, DailyLimit: "0", PerTxLimit: "0", UsedToday: "0"}, nil
		}
	}
	s.rollQuotaDayLocked(row)
	return *row, nil
}

func (s *Service) rollQuotaDayLocked(row *TransferQuota) {
	day := time.Now().UTC().Format("2006-01-02")
	if row.Day != day {
		row.Day = day
		row.UsedToday = "0"
	}
}

// reserveQuota 占用母子划转日额度。
// 有 SQL 时走行锁 CAS，保证跨实例一致；无 SQL 退回进程内锁。
// CAS 与 Hop 仍非同事务，失败必须 releaseQuota。
func (s *Service) reserveQuota(parentID, asset, amount string) error {
	if s != nil && s.sqlOn() {
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
			daily,
				per = cp.DailyLimit, cp.PerTxLimit
		}
		q.mu.Unlock()
		got, err := sqlrepo.NewSubAcct(s.sql).ReserveTransferQuotaCAS(context.Background(), parentID, asset, amount, daily, per)
		if err != nil {
			if errors.Is(err, sqlrepo.ErrConflict) {
				return fmt.Errorf("%w: %v", ErrAmount, err)
			}
			if errors.Is(err, sqlrepo.ErrAmount) {
				return ErrAmount
			}
			return err
		}
		q.mu.Lock()
		q.rows[quotaKey(parentID, asset)] = &TransferQuota{
			ParentID: got.ParentID, Asset: got.Asset, DailyLimit: got.DailyLimit, PerTxLimit: got.PerTxLimit,
			UsedToday: got.UsedToday, Day: got.Day,
		}
		q.mu.Unlock()
		return nil
	}
	q := s.quotas()
	q.mu.Lock()
	key := quotaKey(parentID, asset)
	row := q.rows[key]
	if row == nil {
		if loaded, ok := s.loadQuotaSQL(parentID, asset); ok {
			cp := loaded
			row = &cp
			q.rows[key] = row
		} else {
			q.mu.Unlock()
			return nil
		}
	}
	s.rollQuotaDayLocked(row)
	amt, err := money.Parse(amount)
	if err != nil {
		q.mu.Unlock()
		return ErrAmount
	}
	per, err := money.Parse(row.PerTxLimit)
	if err == nil && per.IsPositive() && amt.Cmp(per) > 0 {
		q.mu.Unlock()
		return fmt.Errorf("%w: per-tx limit %s", ErrAmount, row.PerTxLimit)
	}
	daily,
		err := money.Parse(row.DailyLimit)
	if err != nil || !daily.IsPositive() {
		q.mu.Unlock()
		return nil
	}
	used,
		err := money.ParseBook(row.UsedToday)
	if err != nil {
		used = money.Zero()
	}
	next, err := used.Add(amt)
	if err != nil {
		q.mu.Unlock()
		return err
	}
	if next.Cmp(daily) > 0 {
		q.mu.Unlock()
		return fmt.Errorf("%w: daily transfer quota exceeded", ErrAmount)
	}
	row.UsedToday = next.String()
	out := *row
	q.mu.Unlock()
	return s.persistQuotaSQL(out)
}

func (s *Service) releaseQuota(parentID, asset, amount string) {
	if s != nil && s.sqlOn() {
		got, err := sqlrepo.NewSubAcct(s.sql).ReleaseTransferQuotaCAS(context.Background(), parentID, asset, amount)
		if err == nil && got.ParentID != "" {
			q := s.quotas()
			q.mu.Lock()
			q.rows[quotaKey(parentID, asset)] = &TransferQuota{
				ParentID: got.ParentID,
				Asset:    got.Asset, DailyLimit: got.DailyLimit, PerTxLimit: got.PerTxLimit,
				UsedToday: got.UsedToday, Day: got.Day,
			}
			q.mu.Unlock()
		}
		return
	}
	q := s.quotas()
	q.mu.Lock()
	row := q.rows[quotaKey(parentID, asset)]
	if row == nil {
		q.mu.Unlock()
		return
	}
	s.rollQuotaDayLocked(row)
	amt, err := money.Parse(amount)
	if err != nil {
		q.mu.Unlock()
		return
	}
	used,
		err := money.ParseBook(row.UsedToday)
	if err != nil {
		q.mu.Unlock()
		return
	}
	next, err := used.Sub(amt)
	if err != nil || next.IsNegative() {
		row.UsedToday = "0"
	} else {
		row.UsedToday = next.String()
	}
	out := *row
	q.mu.Unlock()
	_ = s.persistQuotaSQL(out)
}

// FlexTransfer 公开母子划转（PARENT_TO_SUB / SUB_TO_PARENT）。
// 走额度租赁 + Hop（更近同事务/saga）；子到子直接拒。
// 账本/overlay 跨库仍靠租赁与补偿；失败必须释放租赁。

func (s *Service) FlexTransfer(ctx context.Context, typ, parentID, subID, asset, amount, idem string) (HopRec, error) {
	return s.FlexTransferAtomic(ctx, typ, parentID, subID, asset, amount, idem)
}
