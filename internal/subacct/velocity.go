package subacct

import (
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

type Caps struct {
	Hourly   int    `json:"hourly"`
	Daily    int    `json:"daily"`
	DailySum string `json:"daily_sum"`
}

func DefaultCaps() Caps {
	return Caps{Hourly: 8, Daily: 30, DailySum: "20000"}
}

type tick struct {
	at     time.Time
	amount string
}

// Velocity 母子划转频次/日额闸；多实例经 KV/SQL 共享命中。
// 有 SQL 时行锁 CAS；仅 KV 时 fence Put；无持久化退回进程内。
// 多实例必须以 SQL/共享 KV 为准，纯内存会漏限。
type velRevoke struct {
	casID string
	gen   int64
	at    time.Time
}

type Velocity struct {
	mu      sync.Mutex
	caps    Caps
	hits    map[string][]tick
	revoked map[string]velRevoke
	clock   func() time.Time
	kv      persist.KV
	sql     *persist.SQLDB
}

func NewVelocity() *Velocity {
	return &Velocity{
		caps: DefaultCaps(), hits: map[string][]tick{}, revoked: map[string]velRevoke{},
		clock: func() time.Time { return time.Now().UTC() },
	}
}

func (v *Velocity) BindKV(kv persist.KV) *Velocity {
	if v != nil {
		v.kv = kv
	}
	return v
}

func (v *Velocity) BindSQL(db *persist.SQLDB) *Velocity {
	if v != nil {
		v.sql = db
	}
	return v
}

func (v *Velocity) sqlOn() bool {
	return v != nil && v.sql != nil && v.sql.DB != nil
}

func (v *Velocity) SetCaps(c Caps) error {
	if v == nil {
		return ErrUnavailable
	}
	if c.DailySum != "" {
		if _, err := money.Parse(c.DailySum); err != nil {
			return ErrVelocity
		}
	}
	if c.Hourly < 0 || c.Daily < 0 {
		return ErrVelocity
	}
	v.mu.Lock()
	v.caps = c
	v.mu.Unlock()
	return nil
}

// RevokeWindow 吊销冻前 hit：丢弃 FrozenAt 之前的窗口命中，禁止其接着支撑放行。
// 返回被丢弃的 hit 数。
func (v *Velocity) RevokeWindow(owner, casID string, gen int64, at time.Time) int {
	if v == nil {
		return 0
	}
	owner = normalizeOwner(owner)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.revoked == nil {
		v.revoked = map[string]velRevoke{}
	}
	v.revoked[owner] = velRevoke{casID: casID, gen: gen, at: at.UTC()}
	rows := v.hits[owner]
	kept := make([]tick, 0, len(rows))
	n := 0
	for _, h := range rows {
		if !h.at.After(at) {
			n++
			continue
		}
		kept = append(kept, h)
	}
	v.hits[owner] = kept
	return n
}

// AssertNotRevoked 若 owner 速度窗仍处于吊销覆盖则拒绝。
func (v *Velocity) AssertNotRevoked(owner string, now time.Time) error {
	if v == nil {
		return nil
	}
	owner = normalizeOwner(owner)
	v.mu.Lock()
	defer v.mu.Unlock()
	r, ok := v.revoked[owner]
	if !ok || r.at.IsZero() {
		return nil
	}
	if now.Sub(r.at) <= 24*time.Hour {
		return ErrVelocity
	}
	return nil
}

func (v *Velocity) Reserve(owner, amount string) error {
	return v.gate(owner, amount, true)
}

func (v *Velocity) Check(owner, amount string) error {
	return v.gate(owner, amount, false)
}

// ReleaseLast 回滚最近一次 Reserve。余额不足等失败不得占额度。

func (v *Velocity) ReleaseLast(owner string) {
	if v == nil {
		return
	}
	owner = normalizeOwner(owner)
	if v.sqlOn() {
		_ = sqlrepo.NewSubAcct(v.sql).ReleaseVelocityLast(context.Background(), owner)
	}
	if v.kv != nil {
		_ = v.releaseKV(context.Background(), owner)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	rows := v.hits[owner]
	if n := len(rows); n > 0 {
		v.hits[owner] = rows[:n-1]
	}
}

func (v *Velocity) gate(owner, amount string, record bool) error {
	if v == nil {
		return ErrUnavailable
	}
	if _, err := validate.UserID(owner); err != nil && stringsEmpty(owner) {
		return ErrUser
	}
	owner = normalizeOwner(owner)
	amt, err := money.Parse(amount)
	if err != nil {
		return ErrAmount
	}
	now := time.Now().UTC()
	if v.clock != nil {
		now = v.clock()
	}
	if err := v.AssertNotRevoked(owner, now); err != nil {
		return err
	}
	if v.sqlOn() {
		return v.gateSQL(owner, amount, amt, record)
	}
	if v.kv != nil {
		return v.gateKV(owner, amount, amt, record)
	}
	return v.gateMem(owner, amount, amt, record)
}

func (v *Velocity) gateMem(owner, amount string, amt money.Amount, record bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.clock()
	caps := v.caps
	rows := v.hits[owner]
	hour, day, sum, kept, err := foldTicks(rows, now, amt)
	if err != nil {
		return err
	}
	v.hits[owner] = kept
	if err := checkCaps(caps, hour, day, sum); err != nil {
		return err
	}
	if record {
		v.hits[owner] = append(v.hits[owner],
			tick{at: now, amount: amount})
	}
	return nil
}

func (v *Velocity) gateSQL(owner, amount string, amt money.Amount, record bool) error {
	v.mu.Lock()
	caps := v.caps
	v.mu.Unlock()
	if !record {
		rows, err := sqlrepo.NewSubAcct(v.sql).ListVelocityHits(context.Background(), owner, v.clock().Add(-24*time.Hour))
		if err != nil {
			return err
		}
		ticks := make([]tick, 0, len(rows))
		for _, r := range rows {
			ticks = append(ticks, tick{at: r.At, amount: r.Amount})
		}
		hour, day, sum, _, err := foldTicks(ticks, v.clock(), amt)
		if err != nil {
			return err
		}
		return checkCaps(caps, hour, day, sum)
	}
	got, err := sqlrepo.NewSubAcct(v.sql).ReserveVelocityCAS(context.Background(), owner, amount, caps.Hourly, caps.Daily, caps.DailySum, v.clock())
	if err != nil {
		if errors.Is(err, sqlrepo.ErrConflict) {
			return ErrVelocity
		}
		return err
	}
	v.mu.Lock()
	v.hits[owner] = append(v.hits[owner], tick{at: got.At, amount: got.Amount})
	v.mu.Unlock()
	_ = v.persistKVAppend(context.Background(), owner, got.At, got.Amount)
	return nil
}

func (v *Velocity) gateKV(owner, amount string, amt money.Amount, record bool) error {
	ctx := context.Background()
	return v.kv.InTx(ctx, func(ctx context.Context) error {
		cur, fence, err := v.loadKV(ctx, owner)
		if err != nil && !errors.Is(err, persist.ErrNotFound) {
			return err
		}
		expected := persist.Fence{}
		if err == nil {
			expected = fence
		}
		now := v.clock()
		v.mu.Lock()
		caps := v.caps
		v.mu.Unlock()
		hour, day, sum, kept, err := foldTicks(cur, now, amt)
		if err != nil {
			return err
		}
		if err := checkCaps(caps, hour, day, sum); err != nil {
			return err
		}
		if record {
			kept = append(kept,
				tick{at: now, amount: amount})
		}
		if err := v.putKV(ctx, owner, kept, expected); err != nil {
			return err
		}
		v.mu.Lock()
		v.hits[owner] = kept
		v.mu.Unlock()
		return nil
	})
}

const kindVel = "subacct_velocity"

type velBag struct {
	Hits []velHit `json:"hits"`
}

type velHit struct {
	At     time.Time `json:"at"`
	Amount string    `json:"amount"`
}

func velKey(owner string) string { return normalizeOwner(owner) }

func (v *Velocity) loadKV(ctx context.Context, owner string) ([]tick, persist.Fence, error) {
	row, err := v.kv.Get(ctx, kindVel, velKey(owner))
	if err != nil {
		return nil, persist.Fence{}, err
	}
	var b velBag
	if err := json.Unmarshal(row.Body, &b); err != nil {
		return nil, persist.Fence{}, err
	}
	out := make([]tick, 0, len(b.Hits))
	for _, h := range b.Hits {
		out = append(out, tick{at: h.At.UTC(), amount: h.Amount})
	}
	return out, row.Fence, nil
}

func (v *Velocity) putKV(ctx context.Context, owner string, rows []tick, expected persist.Fence) error {
	b := velBag{Hits: make([]velHit, 0, len(rows))}
	for _, h := range rows {
		b.Hits = append(b.Hits, velHit{At: h.at.UTC(), Amount: h.amount})
	}
	body, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return v.kv.Put(ctx, persist.Row{Kind: kindVel, Key: velKey(owner), Body: body}, expected)
}

func (v *Velocity) persistKVAppend(ctx context.Context, owner string, at time.Time, amount string) error {
	if v == nil || v.kv == nil {
		return nil
	}
	return v.kv.InTx(ctx, func(ctx context.Context) error {
		cur, fence, err := v.loadKV(ctx, owner)
		expected := persist.Fence{}
		if err == nil {
			expected = fence
		} else if !errors.Is(err, persist.ErrNotFound) {
			return err
		}
		now := v.clock()
		_,
			_, _, kept, _ := foldTicks(cur, now, money.Zero())
		kept = append(kept, tick{at: at, amount: amount})
		return v.putKV(ctx, owner, kept, expected)
	})
}

func (v *Velocity) releaseKV(ctx context.Context, owner string) error {
	if v == nil || v.kv == nil {
		return nil
	}
	return v.kv.InTx(ctx, func(ctx context.Context) error {
		cur, fence, err := v.loadKV(ctx, owner)
		if errors.Is(err, persist.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if n := len(cur); n > 0 {
			cur = cur[:n-1]
		}
		return v.putKV(ctx, owner, cur, fence)
	})
}

// Hydrate 从 SQL（先）或 KV 装回 24h 内命中。
func (v *Velocity) Hydrate(ctx context.Context) error {
	if v == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if v.sqlOn() {
		rows, err := sqlrepo.NewSubAcct(v.sql).ListVelocityHits(ctx, "", v.clock().Add(-24*time.Hour))
		if err != nil {
			return err
		}
		v.mu.Lock()
		v.hits = map[string][]tick{}
		for _, r := range rows {
			o := normalizeOwner(r.Owner)
			v.hits[o] = append(v.hits[o], tick{at: r.At, amount: r.Amount})
		}
		v.mu.Unlock()
		return nil
	}
	if v.kv == nil {
		return nil
	}
	// KV 全量 ListAfter 游标装回 24h 命中（SQL 已覆盖时不走此支）。
	// 大所级禁止单页截断；与 subacct.hydratePageSize 同游标。
	// 装回后仍以后续 Reserve 的 KV fence 为准，避免陈旧覆盖。
	now := v.clock()
	cut := now.Add(-24 * time.Hour)
	hits := map[string][]tick{}
	after := ""
	for {
		page, err := persist.PageKind(ctx, v.kv, kindVel, persist.Cursor{After: after, Limit: hydratePageSize})
		if err != nil {
			return err
		}
		for _, row := range page.Rows {
			var b velBag
			if err := json.Unmarshal(row.Body, &b); err != nil {
				continue
			}
			o := normalizeOwner(row.Key)
			if o == "" {
				o = normalizeOwner(row.ID)
			}
			kept := make([]tick, 0, len(b.Hits))
			for _, h := range b.Hits {
				at := h.At.UTC()
				if at.Before(cut) {
					continue
				}
				kept = append(kept, tick{at: at, amount: h.Amount})
			}
			if len(kept) > 0 {
				hits[o] = kept
			}
		}
		if page.Done {
			break
		}
		after = page.Next
		if after == "" {
			break
		}
	}
	v.mu.Lock()
	v.hits = hits
	v.mu.Unlock()
	return nil
}

func foldTicks(rows []tick, now time.Time, next money.Amount) (hour, day int, sum money.Amount, kept []tick, err error) {
	sum, _ = money.ParseBook("0")
	kept = rows[:0]
	for _, h := range rows {
		if now.Sub(h.at) > 24*time.Hour {
			continue
		}
		kept = append(kept, h)
		if now.Sub(h.at) <= time.Hour {
			hour++
		}
		day++
		got, e := money.Parse(h.amount)
		if e != nil {
			continue
		}
		sum, _ = sum.Add(got)
	}
	if next.IsPositive() {
		sum, err = sum.Add(next)
		if err != nil {
			return hour, day, sum, kept, ErrVelocity
		}
	}
	return hour, day, sum, kept, nil
}

func checkCaps(caps Caps, hour, day int, sum money.Amount) error {
	if caps.Hourly > 0 && hour >= caps.Hourly {
		return ErrVelocity
	}
	if caps.Daily > 0 && day >= caps.Daily {
		return ErrVelocity
	}
	if caps.DailySum != "" {
		capAmt, err := money.Parse(caps.DailySum)
		if err != nil {
			return ErrVelocity
		}
		if sum.Cmp(capAmt) > 0 {
			return ErrVelocity
		}
	}
	return nil
}

func normalizeOwner(s string) string { return strings.TrimSpace(s) }

func stringsEmpty(s string) bool { return len(s) == 0 }

// ClearRevoke 解冻后解除速度窗吊销标记；冻前 hit 已剔除，新 Reserve 重新计数。
func (v *Velocity) ClearRevoke(owner string) {
	if v == nil {
		return
	}
	owner = normalizeOwner(owner)
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.revoked, owner)
}

func (v *Velocity) Caps() Caps {
	if v == nil {
		return Caps{}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.caps
}
