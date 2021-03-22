package booksnapgate_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/booksnapgate"
)

// ──────────────────────────────────────────
// Mock WaterlineQuerier
// ──────────────────────────────────────────

type mockWaterlineQuerier struct {
	observations	[]booksnapgate.WaterlineObservation
	calls		int
	err		error
	ObserveFunc	func(ctx context.Context, userID, asset string) ([]booksnapgate.WaterlineObservation, error)
}

func newMockWaterlineQuerier(obs []booksnapgate.WaterlineObservation,) *mockWaterlineQuerier {
	return &mockWaterlineQuerier{
		observations: obs,
	}
}

func (m *mockWaterlineQuerier) ObserveWaterlines(ctx context.Context, userID, asset string) ([]booksnapgate.WaterlineObservation, error) {
	m.calls++
	if m.ObserveFunc != nil {
		return m.ObserveFunc(ctx, userID, asset)
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.observations, nil
}

func (m *mockWaterlineQuerier) SetObservations(obs []booksnapgate.WaterlineObservation,) {
	m.observations = obs
}

// ──────────────────────────────────────────
// Mock BillProvider
// ──────────────────────────────────────────

type mockBillProvider struct {
	bills	map[string][]booksnapgate.BillEntry
	err	error
}

func newMockBillProvider() *mockBillProvider {
	return &mockBillProvider{
		bills: make(map[string][]booksnapgate.BillEntry),
	}
}

func (m *mockBillProvider) FetchBills(_ context.Context, _, _, book string, _ int64) ([]booksnapgate.BillEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.bills[book], nil
}

func (m *mockBillProvider) SetBills(book string, bills []booksnapgate.BillEntry,) {
	m.bills[book] = bills
}

// ──────────────────────────────────────────
// 辅助构建
// ──────────────────────────────────────────

func defaultWaterlines() []booksnapgate.WaterlineObservation {
	return []booksnapgate.WaterlineObservation{
		{Book: "funding", Waterline: "100.00", ObserveGen: 1, Checksum: "ck_funding_1"},
		{Book: "trading", Waterline: "50.00", ObserveGen: 1, Checksum: "ck_trading_1"},
		{Book: "margin", Waterline: "25.00", ObserveGen: 1, Checksum: "ck_margin_1"},
		{Book: "futures", Waterline: "10.00", ObserveGen: 1, Checksum: "ck_futures_1"},
		{Book: "earn", Waterline: "5.00", ObserveGen: 1, Checksum: "ck_earn_1"},
	}
}

func conservedBills() map[string][]booksnapgate.BillEntry {
	return map[string][]booksnapgate.BillEntry{
		"funding":	{{Seq: 1, Book: "funding", Asset: "USDT", Debit: "100.00", Credit: "100.00"}},
		"trading":	{{Seq: 2, Book: "trading", Asset: "USDT", Debit: "50.00", Credit: "50.00"}},
		"margin":	{{Seq: 3, Book: "margin", Asset: "USDT", Debit: "25.00", Credit: "25.00"}},
		"futures":	{{Seq: 4, Book: "futures", Asset: "USDT", Debit: "10.00", Credit: "10.00"}},
		"earn":		{{Seq: 5, Book: "earn", Asset: "USDT", Debit: "5.00", Credit: "5.00"}},
	}
}

func unconservedBills() map[string][]booksnapgate.BillEntry {
	return map[string][]booksnapgate.BillEntry{
		"funding":	{{Seq: 1, Book: "funding", Asset: "USDT", Debit: "100.00", Credit: "50.00"}},
		"trading":	{{Seq: 2, Book: "trading", Asset: "USDT", Debit: "50.00", Credit: "30.00"}},
		"margin":	{{Seq: 3, Book: "margin", Asset: "USDT", Debit: "20.00", Credit: "10.00"}},
	}
}

func newTestEngine(wq *mockWaterlineQuerier, bp *mockBillProvider, now time.Time) *booksnapgate.Engine {
	return booksnapgate.New(booksnapgate.Config{
		Store:			booksnapgate.NewMemStore(),
		WaterlineQuerier:	wq,
		BillProvider:		bp,
		Clock:			func() time.Time { return now },
		SnapLeaseTTL:		10 * time.Minute,
		MaxRepin:		3,
	})
}

// ──────────────────────────────────────────
// TestPinSnap table-driven
// ──────────────────────────────────────────

func TestPinSnap(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		asset		string
		wantState	booksnapgate.LeaseState
		wantGen		int64
		wantBooks	int
		wantErr		string
	}{
		{
			name:		"正常 pin 全科目快照",
			userID:		"USER-001",
			asset:		"USDT",
			wantState:	booksnapgate.LeasePinned,
			wantGen:	1,
			wantBooks:	5,
		},
		{
			name:		"BTC 资产 pin",
			userID:		"USER-002",
			asset:		"BTC",
			wantState:	booksnapgate.LeasePinned,
			wantGen:	1,
			wantBooks:	5,
		},
		{
			name:		"小写资产自动转大写",
			userID:		"USER-003",
			asset:		"eth",
			wantState:	booksnapgate.LeasePinned,
			wantGen:	1,
			wantBooks:	5,
		},
		{
			name:		"空用户 ID 拒绝",
			userID:		"",
			asset:		"USDT",
			wantErr:	"user id invalid",
		},
		{
			name:		"空资产拒绝",
			userID:		"USER-004",
			asset:		"",
			wantErr:	"asset invalid",
		},
		{
			name:		"带空格用户 ID 自动 trim",
			userID:		"  USER-005  ",
			asset:		"USDT",
			wantState:	booksnapgate.LeasePinned,
			wantGen:	1,
			wantBooks:	5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wq := newMockWaterlineQuerier(defaultWaterlines())
			eng := newTestEngine(wq, nil, baseTime)
			resp, err := eng.PinSnap(context.Background(), tt.userID, tt.asset, 0, "")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("期望错误包含 %q，但未返回错误", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("错误 %q 不包含 %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望错误: %v", err)
			}
			if resp.State != tt.wantState {
				t.Errorf("state = %s, want %s", resp.State, tt.wantState)
			}
			if resp.SnapGen != tt.wantGen {
				t.Errorf("snap_gen = %d, want %d", resp.SnapGen, tt.wantGen)
			}
			if len(resp.BookSnaps) != tt.wantBooks {
				t.Errorf("book_snaps count = %d, want %d", len(resp.BookSnaps), tt.wantBooks)
			}
			if resp.LeaseID == "" {
				t.Error("lease_id 不应为空")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestPinSnapIdempotent 幂等 pin table-driven
// ──────────────────────────────────────────

func TestPinSnapIdempotent(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	wq := newMockWaterlineQuerier(defaultWaterlines())
	eng := newTestEngine(wq, nil, baseTime)
	tests := []struct {
		name		string
		userID		string
		asset		string
		pinCount	int
		wantReuse	bool
	}{
		{
			name:		"首次 pin 不复用",
			userID:		"USER-IDEM-001",
			asset:		"USDT",
			pinCount:	1,
			wantReuse:	false,
		},
		{
			name:		"第二次 pin 复用",
			userID:		"USER-IDEM-001",
			asset:		"USDT",
			pinCount:	2,
			wantReuse:	true,
		},
		{
			name:		"不同资产不复用",
			userID:		"USER-IDEM-001",
			asset:		"BTC",
			pinCount:	1,
			wantReuse:	false,
		},
		{
			name:		"不同用户不复用",
			userID:		"USER-IDEM-002",
			asset:		"USDT",
			pinCount:	1,
			wantReuse:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lastResp booksnapgate.PinSnapResp
			for i := 0; i < tt.pinCount; i++ {
				resp, err := eng.PinSnap(context.Background(), tt.userID, tt.asset, 0, "")
				if err != nil {
					t.Fatalf("pin %d: %v", i+1, err)
				}
				lastResp = resp
			}
			if lastResp.Reused != tt.wantReuse {
				t.Errorf("reused = %v, want %v", lastResp.Reused, tt.wantReuse)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestAssemble table-driven（核心）
// ──────────────────────────────────────────

func TestAssemble(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		asset		string
		bills		map[string][]booksnapgate.BillEntry
		driftAfterPin	bool
		driftBook	string
		driftWaterline	string
		driftGen	int64
		wantState	booksnapgate.LeaseState
		wantConserved	bool
		wantDrift	bool
		wantDriftKind	booksnapgate.DriftKind
		wantErr		string
	}{
		{
			name:		"正常组装全科目守恒",
			userID:		"USER-ASM-001",
			asset:		"USDT",
			bills:		conservedBills(),
			wantState:	booksnapgate.LeaseSealed,
			wantConserved:	true,
		},
		{
			name:		"不守恒产出 drift proof",
			userID:		"USER-ASM-002",
			asset:		"USDT",
			bills:		unconservedBills(),
			wantState:	booksnapgate.LeaseDrifted,
			wantConserved:	false,
			wantDrift:	true,
			wantDriftKind:	booksnapgate.DriftKindConserveFail,
			wantErr:	"conserve check failed",
		},
		{
			name:		"funding 水线漂移",
			userID:		"USER-ASM-003",
			asset:		"USDT",
			bills:		conservedBills(),
			driftAfterPin:	true,
			driftBook:	"funding",
			driftWaterline:	"200.00",
			driftGen:	2,
			wantState:	booksnapgate.LeaseDrifted,
			wantDrift:	true,
			wantDriftKind:	booksnapgate.DriftKindWaterline,
			wantErr:	"drift detected",
		},
		{
			name:		"trading 水线漂移",
			userID:		"USER-ASM-004",
			asset:		"USDT",
			bills:		conservedBills(),
			driftAfterPin:	true,
			driftBook:	"trading",
			driftWaterline:	"80.00",
			driftGen:	2,
			wantState:	booksnapgate.LeaseDrifted,
			wantDrift:	true,
			wantDriftKind:	booksnapgate.DriftKindWaterline,
			wantErr:	"drift detected",
		},
		{
			name:		"margin 世代漂移",
			userID:		"USER-ASM-005",
			asset:		"USDT",
			bills:		conservedBills(),
			driftAfterPin:	true,
			driftBook:	"margin",
			driftWaterline:	"25.00",
			driftGen:	3,
			wantState:	booksnapgate.LeaseDrifted,
			wantDrift:	true,
			wantDriftKind:	booksnapgate.DriftKindGenMismatch,
			wantErr:	"drift detected",
		},
		{
			name:		"空账单也能封存（零守恒）",
			userID:		"USER-ASM-006",
			asset:		"USDT",
			bills:		map[string][]booksnapgate.BillEntry{},
			wantState:	booksnapgate.LeaseSealed,
			wantConserved:	true,
		},
		{
			name:	"单科目有账单守恒",
			userID:	"USER-ASM-007",
			asset:	"USDT",
			bills: map[string][]booksnapgate.BillEntry{
				"funding": {{Seq: 1, Book: "funding", Asset: "USDT", Debit: "10.00", Credit: "10.00"}},
			},
			wantState:	booksnapgate.LeaseSealed,
			wantConserved:	true,
		},
		{
			name:		"futures 校验和漂移",
			userID:		"USER-ASM-008",
			asset:		"USDT",
			bills:		conservedBills(),
			driftAfterPin:	true,
			driftBook:	"futures",
			driftWaterline:	"10.00",
			driftGen:	1,
			wantState:	booksnapgate.LeaseDrifted,
			wantDrift:	true,
			wantDriftKind:	booksnapgate.DriftKindCreditHop,
			wantErr:	"drift detected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wq := newMockWaterlineQuerier(defaultWaterlines())
			bp := newMockBillProvider()
			for book, bills := range tt.bills {
				bp.SetBills(book, bills)
			}
			eng := newTestEngine(wq, bp, baseTime)
			pin, err := eng.PinSnap(context.Background(), tt.userID, tt.asset, 0, "")
			if err != nil {
				t.Fatalf("pin: %v", err)
			}
			if tt.driftAfterPin {
				obs := make([]booksnapgate.WaterlineObservation, len(defaultWaterlines()))
				copy(obs, defaultWaterlines())
				for i := range obs {
					if obs[i].Book == tt.driftBook {
						obs[i].Waterline = tt.driftWaterline
						if tt.driftGen > 0 {
							obs[i].ObserveGen = tt.driftGen
						}
						if tt.wantDriftKind == booksnapgate.DriftKindCreditHop {
							obs[i].Checksum = "changed_checksum"
						} else {
							obs[i].Checksum = booksnapgate.WaterlineChecksum(tt.userID, tt.asset, tt.driftBook, tt.driftWaterline)
						}
					}
				}
				wq.SetObservations(obs)
			}
			resp, err := eng.Assemble(context.Background(), pin.LeaseID)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("期望错误包含 %q，但未返回错误", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("错误 %q 不包含 %q", err.Error(), tt.wantErr)
				}
			}
			if resp.State != tt.wantState {
				t.Errorf("state = %s, want %s", resp.State, tt.wantState)
			}
			if tt.wantDrift {
				if resp.DriftProof == nil {
					t.Error("期望有 drift proof")
				} else if resp.DriftProof.Kind != tt.wantDriftKind {
					t.Errorf("drift kind = %s, want %s", resp.DriftProof.Kind, tt.wantDriftKind)
				}
			}
			if tt.wantConserved && resp.Result != nil && !resp.Result.Conserved {
				t.Error("期望守恒但 result.Conserved = false")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestMidAssembleCrossBookHop 跨科目跳转拒绝
// ──────────────────────────────────────────

func TestMidAssembleCrossBookHop(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		sourceBook	string
		targetBook	string
		sourceShift	string
		targetShift	string
		wantReject	bool
	}{
		{
			name:		"funding→trading 跳转",
			sourceBook:	"funding",
			targetBook:	"trading",
			sourceShift:	"90.00",
			targetShift:	"60.00",
			wantReject:	true,
		},
		{
			name:		"funding→margin 跳转",
			sourceBook:	"funding",
			targetBook:	"margin",
			sourceShift:	"80.00",
			targetShift:	"45.00",
			wantReject:	true,
		},
		{
			name:		"funding→futures 跳转",
			sourceBook:	"funding",
			targetBook:	"futures",
			sourceShift:	"95.00",
			targetShift:	"15.00",
			wantReject:	true,
		},
		{
			name:		"funding→earn 跳转",
			sourceBook:	"funding",
			targetBook:	"earn",
			sourceShift:	"98.00",
			targetShift:	"7.00",
			wantReject:	true,
		},
		{
			name:		"margin→trading 跳转",
			sourceBook:	"margin",
			targetBook:	"trading",
			sourceShift:	"20.00",
			targetShift:	"55.00",
			wantReject:	true,
		},
		{
			name:		"futures→trading 跳转",
			sourceBook:	"futures",
			targetBook:	"trading",
			sourceShift:	"5.00",
			targetShift:	"55.00",
			wantReject:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wq := newMockWaterlineQuerier(defaultWaterlines())
			bp := newMockBillProvider()
			for book, bills := range conservedBills() {
				bp.SetBills(book, bills)
			}
			eng := newTestEngine(wq, bp, baseTime)
			pin, err := eng.PinSnap(context.Background(), "USER-HOP-"+tt.sourceBook, "USDT", 0, "")
			if err != nil {
				t.Fatalf("pin: %v", err)
			}
			obs := make([]booksnapgate.WaterlineObservation, len(defaultWaterlines()))
			copy(obs, defaultWaterlines())
			for i := range obs {
				if obs[i].Book == tt.sourceBook {
					obs[i].Waterline = tt.sourceShift
					obs[i].ObserveGen = 2
					obs[i].Checksum = "shifted_" + tt.sourceBook
				}
				if obs[i].Book == tt.targetBook {
					obs[i].Waterline = tt.targetShift
					obs[i].ObserveGen = 2
					obs[i].Checksum = "shifted_" + tt.targetBook
				}
			}
			wq.SetObservations(obs)
			resp, err := eng.Assemble(context.Background(), pin.LeaseID)
			if tt.wantReject {
				if err == nil {
					t.Fatal("期望组装被拒绝")
				}
				if resp.State != booksnapgate.LeaseDrifted {
					t.Errorf("state = %s, want drifted", resp.State)
				}
				if resp.DriftProof == nil {
					t.Error("期望有 drift proof")
				}
				if resp.Result != nil {
					t.Error("漂移时不应返回 conserve result")
				}
			}
		})
	}
}

// ──────────────────────────────────────────
// TestExpiredSnapCannotContinue 过期租约拒绝
// ──────────────────────────────────────────

func TestExpiredSnapCannotContinue(t *testing.T) {
	tests := []struct {
		name		string
		pinTime		time.Time
		assembleAt	time.Time
		ttl		time.Duration
		wantExpired	bool
	}{
		{
			name:		"TTL 内可组装",
			pinTime:	time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC),
			assembleAt:	time.Date(2021, 11, 22, 10, 5, 0, 0, time.UTC),
			ttl:		10 * time.Minute,
			wantExpired:	false,
		},
		{
			name:		"TTL 过期拒绝",
			pinTime:	time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC),
			assembleAt: time.Date(2021,
				11, 22, 10, 15, 0, 0, time.UTC),
			ttl:		10 * time.Minute,
			wantExpired:	true,
		},
		{
			name:		"刚好过期",
			pinTime:	time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC),
			assembleAt:	time.Date(2021, 11, 22, 10, 10, 1, 0, time.UTC),
			ttl:		10 * time.Minute,
			wantExpired:	true,
		},
		{
			name:		"1 秒 TTL",
			pinTime:	time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC),
			assembleAt:	time.Date(2021, 11, 22, 10, 0, 2, 0, time.UTC),
			ttl:		1 * time.Second,
			wantExpired:	true,
		},
		{
			name:	"1 小时 TTL 未过期",
			pinTime: time.Date(2021,
				11, 22, 10, 0, 0, 0, time.UTC),
			assembleAt:	time.Date(2021, 11, 22, 10, 30, 0, 0, time.UTC),
			ttl:		1 * time.Hour,
			wantExpired:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentTime := tt.pinTime
			wq := newMockWaterlineQuerier(defaultWaterlines())
			bp := newMockBillProvider()
			for book, bills := range conservedBills() {
				bp.SetBills(book, bills)
			}
			eng := booksnapgate.New(booksnapgate.Config{
				Store:			booksnapgate.NewMemStore(),
				WaterlineQuerier:	wq,
				BillProvider:		bp,
				Clock:			func() time.Time { return currentTime },
				SnapLeaseTTL:		tt.ttl,
			})
			pin, err := eng.PinSnap(context.Background(), "USER-EXP", "USDT", tt.ttl, "")
			if err != nil {
				t.Fatalf("pin: %v", err)
			}
			currentTime = tt.assembleAt
			eng.clock = func() time.Time { return currentTime }
			_, err = eng.Assemble(context.Background(), pin.LeaseID)
			if tt.wantExpired {
				if err == nil {
					t.Fatal("期望过期错误")
				}
				if !errors.Is(err, booksnapgate.ErrLeaseExpired) {
					t.Errorf("错误 = %v, want ErrLeaseExpired", err)
				}
			} else {
				if err != nil {
					t.Fatalf("不期望错误: %v", err)
				}
			}
		})
	}
}

// ──────────────────────────────────────────
// TestIdempotentBuild table-driven
// ──────────────────────────────────────────

func TestIdempotentBuild(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	window := "USER-IDEM|USDT|2021-11-22T10:00:00Z|2021-11-22T11:00:00Z"
	tests := []struct {
		name		string
		callCount	int
		wantReused	[]bool
		wantState	booksnapgate.LeaseState
	}{
		{
			name:		"首次 build",
			callCount:	1,
			wantReused:	[]bool{false},
			wantState:	booksnapgate.LeaseSealed,
		},
		{
			name:		"第二次 build 复用",
			callCount:	2,
			wantReused:	[]bool{false, true},
			wantState:	booksnapgate.LeaseSealed,
		},
		{
			name:		"第三次 build 仍复用",
			callCount:	3,
			wantReused:	[]bool{false, true, true},
			wantState:	booksnapgate.LeaseSealed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wq := newMockWaterlineQuerier(defaultWaterlines())
			bp := newMockBillProvider()
			for book, bills := range conservedBills() {
				bp.SetBills(book, bills)
			}
			eng := newTestEngine(wq, bp, baseTime)
			for i := 0; i < tt.callCount; i++ {
				resp, err := eng.IdempotentBuild(context.Background(), "USER-IDEM", "USDT", window)
				if err != nil {
					t.Fatalf("build %d: %v", i+1, err)
				}
				if resp.State != tt.wantState {
					t.Errorf("build %d: state = %s, want %s", i+1, resp.State, tt.wantState)
				}
				if i < len(tt.wantReused) && resp.Reused != tt.wantReused[i] {
					t.Errorf("build %d: reused = %v, want %v", i+1, resp.Reused, tt.wantReused[i])
				}
			}
		})
	}
}

// ──────────────────────────────────────────
// TestRePin table-driven
// ──────────────────────────────────────────

func TestRePin(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		maxRepin	int
		driftCount	int
		repinCount	int
		wantLastState	booksnapgate.LeaseState
		wantErr		string
	}{
		{
			name:		"单次漂移 re-pin",
			maxRepin:	3,
			driftCount:	1,
			repinCount:	1,
			wantLastState:	booksnapgate.LeasePinned,
		},
		{
			name:		"两次漂移两次 re-pin",
			maxRepin:	3,
			driftCount:	2,
			repinCount:	2,
			wantLastState:	booksnapgate.LeasePinned,
		},
		{
			name:		"超过最大 re-pin",
			maxRepin:	2,
			driftCount:	3,
			repinCount:	3,
			wantLastState:	booksnapgate.LeaseDrifted,
			wantErr:	"max repin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wq := newMockWaterlineQuerier(defaultWaterlines())
			bp := newMockBillProvider()
			eng := booksnapgate.New(booksnapgate.Config{
				Store:			booksnapgate.NewMemStore(),
				WaterlineQuerier:	wq,
				BillProvider:		bp,
				Clock:			func() time.Time { return baseTime },
				SnapLeaseTTL:		10 * time.Minute,
				MaxRepin:		tt.maxRepin,
			})
			pin, err := eng.PinSnap(context.Background(), "USER-REPIN", "USDT", 0, "")
			if err != nil {
				t.Fatalf("pin: %v", err)
			}
			leaseID := pin.LeaseID
			var lastErr error
			for i := 0; i < tt.driftCount; i++ {
				drifted := make([]booksnapgate.WaterlineObservation, len(defaultWaterlines()))
				copy(drifted,
					defaultWaterlines())
				drifted[0].Waterline = fmt.Sprintf("%d.00", 100+i+1)
				drifted[0].ObserveGen = int64(2 + i)
				drifted[0].Checksum = fmt.Sprintf("drift_%d", i)
				wq.SetObservations(drifted)
				_, _ = eng.Assemble(context.Background(), leaseID)
				wq.SetObservations(defaultWaterlines())
				if i < tt.repinCount {
					_, lastErr = eng.RePin(context.Background(), leaseID)
				}
			}
			if tt.wantErr != "" {
				if lastErr == nil {
					t.Fatalf("期望错误包含 %q", tt.wantErr)
				}
				if !strings.Contains(lastErr.Error(), tt.wantErr) {
					t.Fatalf("错误 %q 不包含 %q", lastErr.Error(), tt.wantErr)
				}
			}
			lease, _ := eng.GetLease(context.Background(), leaseID)
			if lease.State != tt.wantLastState {
				t.Errorf("final state = %s, want %s", lease.State, tt.wantLastState)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestAbort table-driven
// ──────────────────────────────────────────

func TestAbort(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		preAssemble	bool
		reason		string
		wantState	booksnapgate.LeaseState
		wantProofKind	booksnapgate.DriftKind
	}{
		{
			name:		"pinned 状态中止",
			preAssemble:	false,
			reason:		"运维中止",
			wantState:	booksnapgate.LeaseAborted,
			wantProofKind:	booksnapgate.DriftKindAbort,
		},
		{
			name:		"assembling 状态中止",
			preAssemble:	true,
			reason:		"超时中止",
			wantState:	booksnapgate.LeaseAborted,
			wantProofKind:	booksnapgate.DriftKindAbort,
		},
		{
			name:		"无理由中止",
			preAssemble:	false,
			reason:		"",
			wantState:	booksnapgate.LeaseAborted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wq := newMockWaterlineQuerier(defaultWaterlines())
			eng := newTestEngine(wq, nil, baseTime)
			pin, err := eng.PinSnap(context.Background(), "USER-ABORT", "USDT", 0, "")
			if err != nil {
				t.Fatalf("pin: %v", err)
			}
			if tt.preAssemble {
				if err := eng.BeginAssemble(context.Background(), pin.LeaseID); err != nil {
					t.Fatalf("begin assemble: %v", err)
				}
			}
			if err := eng.Abort(context.Background(), pin.LeaseID, tt.reason); err != nil {
				t.Fatalf("abort: %v", err)
			}
			lease, _ := eng.GetLease(context.Background(), pin.LeaseID)
			if lease.State != tt.wantState {
				t.Errorf("state = %s, want %s", lease.State, tt.wantState)
			}
			if tt.reason != "" {
				proofs, _ := eng.ExportProofs(context.Background(), pin.LeaseID, "", 10)
				found := false
				for _, p := range proofs {
					if p.Kind == tt.wantProofKind {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("未找到 %s 类型的证明", tt.wantProofKind)
				}
			}
		})
	}
}

// ──────────────────────────────────────────
// TestFSMTransitions table-driven
// ──────────────────────────────────────────

func TestFSMTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	string
		event	string
		wantTo	string
		wantErr	bool
	}{
		{"pinned→assembling", string(booksnapgate.LeasePinned), booksnapgate.LeaseEvBeginAssemble, string(booksnapgate.LeaseAssembling), false},
		{"pinned→expired", string(booksnapgate.LeasePinned), booksnapgate.LeaseEvExpire, string(booksnapgate.LeaseExpired), false},
		{"pinned→aborted", string(booksnapgate.LeasePinned), booksnapgate.LeaseEvAbort, string(booksnapgate.LeaseAborted), false},
		{"assembling→sealed", string(booksnapgate.LeaseAssembling), booksnapgate.LeaseEvSeal, string(booksnapgate.LeaseSealed), false},
		{"assembling→drifted", string(booksnapgate.LeaseAssembling), booksnapgate.LeaseEvDrift, string(booksnapgate.LeaseDrifted), false},
		{"assembling→aborted", string(booksnapgate.LeaseAssembling), booksnapgate.LeaseEvAbort, string(booksnapgate.LeaseAborted), false},
		{"assembling→expired", string(booksnapgate.LeaseAssembling), booksnapgate.LeaseEvExpire, string(booksnapgate.LeaseExpired), false},
		{"drifted→pinned", string(booksnapgate.LeaseDrifted), booksnapgate.LeaseEvRepin, string(booksnapgate.LeasePinned), false},
		{"drifted→aborted", string(booksnapgate.LeaseDrifted), booksnapgate.LeaseEvAbort, string(booksnapgate.LeaseAborted), false},
		{"drifted→expired", string(booksnapgate.LeaseDrifted), booksnapgate.LeaseEvExpire, string(booksnapgate.LeaseExpired), false},
		{"sealed 终态不可转换", string(booksnapgate.LeaseSealed), booksnapgate.LeaseEvBeginAssemble, "", true},
		{"expired 终态不可转换", string(booksnapgate.LeaseExpired), booksnapgate.LeaseEvRepin, "", true},
		{"aborted 终态不可转换", string(booksnapgate.LeaseAborted), booksnapgate.LeaseEvRepin, "", true},
		{"pinned 不可 seal", string(booksnapgate.LeasePinned), booksnapgate.LeaseEvSeal, "", true},
		{"pinned 不可 drift", string(booksnapgate.LeasePinned), booksnapgate.LeaseEvDrift, "", true},
		{"pinned 不可 repin", string(booksnapgate.LeasePinned), booksnapgate.LeaseEvRepin, "", true},
	}
	fsm := booksnapgate.LeaseStateMachine()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fsm.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望错误但成功转换到 %s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("转换失败: %v", err)
			}
			if got != tt.wantTo {
				t.Errorf("转换结果 = %s, want %s", got, tt.wantTo)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestConserveCheck table-driven
// ──────────────────────────────────────────

func TestConserveCheck(t *testing.T) {
	tests := []struct {
		name		string
		bills		[]booksnapgate.BillEntry
		wantConserved	bool
		wantDebit	string
		wantCredit	string
	}{
		{
			name:		"完全守恒",
			bills:		allBillsFlat(conservedBills()),
			wantConserved:	true,
			wantDebit:	"190",
			wantCredit:	"190",
		},
		{
			name:		"不守恒",
			bills:		allBillsFlat(unconservedBills()),
			wantConserved:	false,
			wantDebit:	"170",
			wantCredit:	"90",
		},
		{
			name:		"空账单守恒",
			bills:		nil,
			wantConserved:	true,
			wantDebit:	"0",
			wantCredit:	"0",
		},
		{
			name:	"单科目守恒",
			bills: []booksnapgate.BillEntry{
				{Book: "funding", Debit: "100", Credit: "100"},
			},
			wantConserved:	true,
			wantDebit:	"100",
			wantCredit:	"100",
		},
		{
			name:	"多科目分散守恒",
			bills: []booksnapgate.BillEntry{
				{Book: "funding", Debit: "50", Credit: "30"},
				{Book: "trading", Debit: "30", Credit: "50"},
			},
			wantConserved:	true,
			wantDebit:	"80",
			wantCredit:	"80",
		},
		{
			name:	"精度守恒",
			bills: []booksnapgate.BillEntry{
				{Book: "funding", Debit: "0.00000001", Credit: "0.00000001"},
			},
			wantConserved:	true,
		},
		{
			name:	"大额守恒",
			bills: []booksnapgate.BillEntry{
				{Book: "funding", Debit: "999999999.99999999", Credit: "999999999.99999999"},
			},
			wantConserved:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := booksnapgate.CheckConserve(tt.bills)
			if result.Err != nil {
				t.Fatalf("错误: %v", result.Err)
			}
			if result.Conserved != tt.wantConserved {
				t.Errorf("conserved = %v, want %v", result.Conserved, tt.wantConserved)
			}
			if tt.wantDebit != "" && result.TotalDebit.String() != tt.wantDebit {
				t.Errorf("total debit = %s, want %s", result.TotalDebit.String(), tt.wantDebit)
			}
			if tt.wantCredit != "" && result.TotalCredit.String() != tt.wantCredit {
				t.Errorf("total credit = %s, want %s", result.TotalCredit.String(), tt.wantCredit)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestDriftGuard table-driven
// ──────────────────────────────────────────

func TestDriftGuard(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		pinSnaps	[]booksnapgate.BookSnap
		currentObs	[]booksnapgate.WaterlineObservation
		wantDrift	bool
		wantKind	booksnapgate.DriftKind
		wantBook	string
	}{
		{
			name:	"无漂移",
			pinSnaps: []booksnapgate.BookSnap{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
				{Book: "trading", Waterline: "50", ObserveGen: 1, Checksum: "c2"},
			},
			currentObs: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
				{Book: "trading", Waterline: "50", ObserveGen: 1, Checksum: "c2"},
			},
			wantDrift:	false,
		},
		{
			name:	"funding 水线漂移",
			pinSnaps: []booksnapgate.BookSnap{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
			},
			currentObs: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "110", ObserveGen: 1, Checksum: "c1"},
			},
			wantDrift:	true,
			wantKind:	booksnapgate.DriftKindWaterline,
			wantBook:	"funding",
		},
		{
			name:	"世代漂移",
			pinSnaps: []booksnapgate.BookSnap{
				{Book: "trading", Waterline: "50", ObserveGen: 1, Checksum: "c1"},
			},
			currentObs: []booksnapgate.WaterlineObservation{
				{Book: "trading", Waterline: "50", ObserveGen: 2, Checksum: "c1"},
			},
			wantDrift:	true,
			wantKind:	booksnapgate.DriftKindGenMismatch,
			wantBook:	"trading",
		},
		{
			name:	"校验和漂移",
			pinSnaps: []booksnapgate.BookSnap{
				{Book: "margin", Waterline: "25", ObserveGen: 1, Checksum: "c1"},
			},
			currentObs: []booksnapgate.WaterlineObservation{
				{Book: "margin", Waterline: "25", ObserveGen: 1, Checksum: "c2"},
			},
			wantDrift:	true,
			wantKind:	booksnapgate.DriftKindCreditHop,
			wantBook:	"margin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wq := newMockWaterlineQuerier(tt.currentObs)
			lease := booksnapgate.BookSnapLease{
				ID:		"test-lease",
				UserID:		"USER-DG",
				Asset:		"USDT",
				SnapGen:	1,
				BookSnaps:	tt.pinSnaps,
			}
			guard := booksnapgate.NewDriftGuard(lease,
				wq, func() time.Time { return baseTime })
			drifted, proof, err := guard.Check(context.Background())
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if drifted != tt.wantDrift {
				t.Errorf("drifted = %v, want %v", drifted, tt.wantDrift)
			}
			if tt.wantDrift {
				if proof == nil {
					t.Fatal("期望有 proof")
				}
				if proof.Kind != tt.wantKind {
					t.Errorf("kind = %s, want %s", proof.Kind, tt.wantKind)
				}
				if proof.Book != tt.wantBook {
					t.Errorf("book = %s, want %s", proof.Book, tt.wantBook)
				}
			}
		})
	}
}

// ──────────────────────────────────────────
// TestWaterlineDiff table-driven
// ──────────────────────────────────────────

func TestWaterlineDiff(t *testing.T) {
	tests := []struct {
		name		string
		pinned		[]booksnapgate.WaterlineObservation
		current		[]booksnapgate.WaterlineObservation
		wantDiffs	int
		wantKinds	[]booksnapgate.DiffKind
	}{
		{
			name:	"无差异",
			pinned: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
			},
			current: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
			},
			wantDiffs:	0,
		},
		{
			name:	"水线差异",
			pinned: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
			},
			current: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "110", ObserveGen: 1, Checksum: "c1"},
			},
			wantDiffs:	1,
			wantKinds:	[]booksnapgate.DiffKind{booksnapgate.DiffKindWaterline},
		},
		{
			name:	"世代差异",
			pinned: []booksnapgate.WaterlineObservation{
				{Book: "trading", Waterline: "50", ObserveGen: 1, Checksum: "c1"},
			},
			current: []booksnapgate.WaterlineObservation{
				{Book: "trading", Waterline: "50", ObserveGen: 2, Checksum: "c1"},
			},
			wantDiffs:	1,
			wantKinds:	[]booksnapgate.DiffKind{booksnapgate.DiffKindGen},
		},
		{
			name:	"校验和差异",
			pinned: []booksnapgate.WaterlineObservation{
				{Book: "margin", Waterline: "25", ObserveGen: 1, Checksum: "c1"},
			},
			current: []booksnapgate.WaterlineObservation{
				{Book: "margin", Waterline: "25", ObserveGen: 1, Checksum: "c2"},
			},
			wantDiffs:	1,
			wantKinds:	[]booksnapgate.DiffKind{booksnapgate.DiffKindChecksum},
		},
		{
			name:	"新科目",
			pinned: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
			},
			current: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
				{Book: "trading", Waterline: "50", ObserveGen: 1, Checksum: "c2"},
			},
			wantDiffs:	1,
			wantKinds:	[]booksnapgate.DiffKind{booksnapgate.DiffKindNew},
		},
		{
			name:	"多维度差异",
			pinned: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "100", ObserveGen: 1, Checksum: "c1"},
				{Book: "trading", Waterline: "50", ObserveGen: 1, Checksum: "c2"},
			},
			current: []booksnapgate.WaterlineObservation{
				{Book: "funding", Waterline: "110", ObserveGen: 1, Checksum: "c1"},
				{Book: "trading", Waterline: "50", ObserveGen: 2, Checksum: "c2"},
			},
			wantDiffs:	2,
			wantKinds:	[]booksnapgate.DiffKind{booksnapgate.DiffKindWaterline, booksnapgate.DiffKindGen},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pinned := booksnapgate.NewWaterlineSet("USER", "USDT", tt.pinned)
			current := booksnapgate.NewWaterlineSet("USER", "USDT", tt.current)
			diffs := booksnapgate.DiffWaterlines(pinned, current)
			if len(diffs) != tt.wantDiffs {
				t.Errorf("diff count = %d, want %d", len(diffs), tt.wantDiffs)
			}
			for i, wk := range tt.wantKinds {
				if i < len(diffs) && diffs[i].Kind != wk {
					t.Errorf("diff[%d].kind = %s, want %s", i, diffs[i].Kind, wk)
				}
			}
		})
	}
}

// ──────────────────────────────────────────
// TestValidateLease table-driven
// ──────────────────────────────────────────

func TestValidateLease(t *testing.T) {
	baseTime := time.Date(2021,
		11, 22, 10, 0, 0, 0, time.UTC)
	sealedAt := baseTime.Add(5 * time.Minute)
	driftedAt := baseTime.Add(3 * time.Minute)
	tests := []struct {
		name	string
		lease	booksnapgate.BookSnapLease
		wantErr	string
	}{
		{
			name:	"有效 pinned 租约",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeasePinned, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
		},
		{
			name:	"空 ID",
			lease: booksnapgate.BookSnapLease{
				UserID:	"U-001", Asset: "USDT", State: booksnapgate.LeasePinned, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
			wantErr:	"lease id empty",
		},
		{
			name:	"空用户",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", Asset: "USDT", State: booksnapgate.LeasePinned, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
			wantErr:	"user id invalid",
		},
		{
			name:	"空资产",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", State: booksnapgate.LeasePinned, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
			wantErr:	"asset invalid",
		},
		{
			name:	"snap_gen 为零",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeasePinned, SnapGen: 0,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
			wantErr:	"snap gen must be positive",
		},
		{
			name:	"无科目快照",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeasePinned, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
			},
			wantErr:	"no book snaps",
		},
		{
			name:	"过期早于 pin",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeasePinned, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(-1 * time.Minute),
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
			wantErr:	"expires_at before pinned_at",
		},
		{
			name:	"sealed 缺少 sealed_at",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeaseSealed, SnapGen: 1,
				PinnedAt:	baseTime,
				ExpiresAt:	baseTime.Add(10 * time.Minute),
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
			wantErr:	"sealed lease missing sealed_at",
		},
		{
			name:	"sealed 有 sealed_at",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeaseSealed, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
				SealedAt:	&sealedAt,
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
		},
		{
			name:	"drifted 缺少 drifted_at",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeaseDrifted, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
			wantErr:	"drifted lease missing drifted_at",
		},
		{
			name:	"drifted 有 drifted_at",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeaseDrifted, SnapGen: 1,
				PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
				DriftedAt:	&driftedAt,
				BookSnaps:	[]booksnapgate.BookSnap{{Book: "funding", SnapID: "S-001"}},
			},
		},
		{
			name:	"重复科目",
			lease: booksnapgate.BookSnapLease{
				ID:	"L-001", UserID: "U-001", Asset: "USDT", State: booksnapgate.LeasePinned, SnapGen: 1,
				PinnedAt:	baseTime,
				ExpiresAt:	baseTime.Add(10 * time.Minute),
				BookSnaps: []booksnapgate.BookSnap{
					{Book: "funding", SnapID: "S-001"},
					{Book: "funding", SnapID: "S-002"},
				},
			},
			wantErr:	"duplicate book snap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := booksnapgate.ValidateLease(tt.lease)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("期望错误包含 %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("错误 %q 不包含 %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望错误: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestHealth
// ──────────────────────────────────────────

func TestHealth(t *testing.T) {
	baseTime := time.Date(2021,
		11, 22, 10, 0, 0, 0, time.UTC)
	wq := newMockWaterlineQuerier(defaultWaterlines())
	bp := newMockBillProvider()
	for book, bills := range conservedBills() {
		bp.SetBills(book, bills)
	}
	eng := newTestEngine(wq, bp, baseTime)
	_, _ = eng.PinSnap(context.Background(), "U-H1", "USDT", 0, "w1")
	_, _ = eng.PinSnap(context.Background(), "U-H2", "BTC", 0, "w2")
	pin3, _ := eng.PinSnap(context.Background(), "U-H3", "ETH", 0, "w3")
	_, _ = eng.Assemble(context.Background(), pin3.LeaseID)
	h,
		err := eng.Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if h.ActiveLeases < 2 {
		t.Errorf("active leases = %d, want >= 2", h.ActiveLeases)
	}
	if h.SealedLeases < 1 {
		t.Errorf("sealed leases = %d, want >= 1", h.SealedLeases)
	}
}

// ──────────────────────────────────────────
// TestSnapWindow table-driven
// ──────────────────────────────────────────

func TestSnapWindow(t *testing.T) {
	tests := []struct {
		name	string
		window	booksnapgate.SnapWindow
		valid	bool
	}{
		{
			name:	"有效窗口",
			window:	booksnapgate.SnapWindow{UserID: "U1", Asset: "USDT", From: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC)},
			valid:	true,
		},
		{
			name:	"缺少用户",
			window:	booksnapgate.SnapWindow{Asset: "USDT", From: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC)},
			valid:	false,
		},
		{
			name:	"结束早于开始",
			window:	booksnapgate.SnapWindow{UserID: "U1", Asset: "USDT", From: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC), To: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
			valid:	false,
		},
		{
			name:	"零时间",
			window:	booksnapgate.SnapWindow{UserID: "U1", Asset: "USDT"},
			valid:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.window.Valid(); got != tt.valid {
				t.Errorf("valid = %v, want %v", got, tt.valid)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestIsHopPair table-driven
// ──────────────────────────────────────────

func TestIsHopPair(t *testing.T) {
	tests := []struct {
		a, b	string
		want	bool
	}{
		{"funding", "trading", true},
		{"trading", "funding", true},
		{"funding", "margin", true},
		{"funding", "futures", true},
		{"funding", "earn", true},
		{"margin", "trading", true},
		{"futures", "trading", true},
		{"earn", "trading", false},
		{"margin", "futures", false},
		{"earn", "margin", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"→"+tt.b, func(t *testing.T) {
			if got := booksnapgate.IsHopPair(tt.a, tt.b); got != tt.want {
				t.Errorf("IsHopPair(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestTryBuildWithRepin table-driven
// ──────────────────────────────────────────

func TestTryBuildWithRepin(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		driftRounds	int
		maxRetries	int
		wantState	booksnapgate.LeaseState
		wantErr		string
	}{
		{
			name:		"无漂移直接封存",
			driftRounds:	0,
			maxRetries:	3,
			wantState:	booksnapgate.LeaseSealed,
		},
		{
			name:		"一次漂移后成功",
			driftRounds:	1,
			maxRetries:	3,
			wantState:	booksnapgate.LeaseSealed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			origObs := defaultWaterlines()
			wq := newMockWaterlineQuerier(origObs)
			wq.ObserveFunc = func(_ context.Context, _, _ string) ([]booksnapgate.WaterlineObservation, error) {
				callCount++
				if callCount <= tt.driftRounds*2+1 && callCount > 1 {
					drift := make([]booksnapgate.WaterlineObservation, len(origObs))
					copy(drift, origObs)
					drift[0].Waterline = fmt.Sprintf("%d.00", 100+callCount)
					drift[0].ObserveGen = int64(callCount)
					drift[0].Checksum = fmt.Sprintf("drift_%d", callCount)
					return drift, nil
				}
				return origObs, nil
			}
			bp := newMockBillProvider()
			for book, bills := range conservedBills() {
				bp.SetBills(book, bills)
			}
			eng := booksnapgate.New(booksnapgate.Config{
				Store:			booksnapgate.NewMemStore(),
				WaterlineQuerier:	wq,
				BillProvider:		bp,
				Clock:			func() time.Time { return baseTime },
				SnapLeaseTTL:		10 * time.Minute,
				MaxRepin:		tt.maxRetries,
			})
			resp, err := eng.TryBuildWithRepin(context.Background(), "USER-RETRY", "USDT", "w-retry", tt.maxRetries)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("期望错误包含 %q", tt.wantErr)
				}
				return
			}
			if err != nil && !booksnapgate.IsRetryable(err) {
				t.Fatalf("不期望非重试错误: %v", err)
			}
			if resp.State != tt.wantState && err == nil {
				t.Errorf("state = %s, want %s", resp.State, tt.wantState)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestStoreOperations table-driven
// ──────────────────────────────────────────

func TestStoreOperations(t *testing.T) {
	baseTime := time.Date(2021,
		11, 22, 10, 0, 0, 0, time.UTC)
	t.Run("SaveAndGetLease", func(t *testing.T) {
		store := booksnapgate.NewMemStore()
		lease := booksnapgate.BookSnapLease{
			ID:	"L-STORE-001", UserID: "U-001", Asset: "USDT",
			State:	booksnapgate.LeasePinned, SnapGen: 1, CreatedAt: baseTime,
			PinnedAt:	baseTime, ExpiresAt: baseTime.Add(10 * time.Minute),
		}
		saved, err := store.SaveLease(context.Background(), lease)
		if err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := store.GetLease(context.Background(), saved.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.UserID != "U-001" {
			t.Errorf("user_id = %s", got.UserID)
		}
	})
	t.Run("GetLease_NotFound", func(t *testing.T) {
		store := booksnapgate.NewMemStore()
		_, err := store.GetLease(context.Background(), "nonexistent")
		if !errors.Is(err, booksnapgate.ErrLeaseNotFound) {
			t.Errorf("err = %v, want ErrLeaseNotFound", err)
		}
	})
	t.Run("SaveAndGetDriftProof", func(t *testing.T) {
		store := booksnapgate.NewMemStore()
		proof := booksnapgate.BookSnapDriftProof{
			ID:	"P-001", LeaseID: "L-001", UserID: "U-001",
			Kind:	booksnapgate.DriftKindWaterline, CreatedAt: baseTime,
		}
		saved, err := store.SaveDriftProof(context.Background(), proof)
		if err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := store.GetDriftProof(context.Background(), saved.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Kind != booksnapgate.DriftKindWaterline {
			t.Errorf("kind = %s", got.Kind)
		}
	})
	t.Run("ListLeasesByState", func(t *testing.T) {
		store := booksnapgate.NewMemStore()
		for i := 0; i < 5; i++ {
			_, _ = store.SaveLease(context.Background(), booksnapgate.BookSnapLease{
				ID:	fmt.Sprintf("L-%d", i), UserID: "U-001", Asset: "USDT",
				State:	booksnapgate.LeasePinned, SnapGen: int64(i + 1), CreatedAt: baseTime.Add(time.Duration(i) * time.Minute),
			})
		}
		for i := 5; i < 8; i++ {
			_, _ = store.SaveLease(context.Background(), booksnapgate.BookSnapLease{
				ID:	fmt.Sprintf("L-%d", i), UserID: "U-001", Asset: "USDT",
				State:	booksnapgate.LeaseSealed, SnapGen: int64(i + 1), CreatedAt: baseTime.Add(time.Duration(i) * time.Minute),
			})
		}
		pinned, _ := store.ListLeasesByState(context.Background(), booksnapgate.LeasePinned, 100)
		sealed, _ := store.ListLeasesByState(context.Background(), booksnapgate.LeaseSealed, 100)
		if len(pinned) != 5 {
			t.Errorf("pinned count = %d, want 5", len(pinned))
		}
		if len(sealed) != 3 {
			t.Errorf("sealed count = %d, want 3", len(sealed))
		}
	})
	t.Run("CountByState", func(t *testing.T) {
		store := booksnapgate.NewMemStore()
		for i := 0; i < 3; i++ {
			_, _ = store.SaveLease(context.Background(), booksnapgate.BookSnapLease{
				ID:	fmt.Sprintf("LC-%d", i), State: booksnapgate.LeasePinned,
			})
		}
		count,
			_ := store.CountByState(context.Background(), booksnapgate.LeasePinned)
		if count != 3 {
			t.Errorf("count = %d, want 3", count)
		}
	})
}

// ──────────────────────────────────────────
// TestValidateMultiConserve table-driven
// ──────────────────────────────────────────

func TestValidateMultiConserve(t *testing.T) {
	tests := []struct {
		name	string
		mc	booksnapgate.MultiConserve
		wantErr	string
	}{
		{
			name:	"有效守恒",
			mc: booksnapgate.MultiConserve{
				TotalDebit:	"100",
				TotalCredit:	"100",
				Conserved:	true,
				Books: []booksnapgate.BookTotals{
					{Book: "funding", Debit: "60", Credit: "60"},
					{Book: "trading", Debit: "40", Credit: "40"},
				},
			},
		},
		{
			name:	"有效不守恒",
			mc: booksnapgate.MultiConserve{
				TotalDebit:	"100",
				TotalCredit:	"80",
				Conserved:	false,
				Delta:		"20",
				Books: []booksnapgate.BookTotals{
					{Book: "funding", Debit: "60", Credit: "50"},
					{Book: "trading", Debit: "40", Credit: "30"},
				},
			},
		},
		{
			name:	"debit 合计不一致",
			mc: booksnapgate.MultiConserve{
				TotalDebit:	"100",
				TotalCredit:	"100",
				Conserved:	true,
				Books: []booksnapgate.BookTotals{
					{Book: "funding", Debit: "60", Credit: "60"},
					{Book: "trading", Debit: "50", Credit: "40"},
				},
			},
			wantErr:	"debit sum",
		},
		{
			name:	"标记守恒但有 delta",
			mc: booksnapgate.MultiConserve{
				TotalDebit:	"100",
				TotalCredit:	"80",
				Conserved:	true,
				Books: []booksnapgate.BookTotals{
					{Book: "funding", Debit: "100", Credit: "80"},
				},
			},
			wantErr:	"marked conserved but delta",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := booksnapgate.ValidateMultiConserve(tt.mc)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("期望错误包含 %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("错误 %q 不包含 %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望错误: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestNilEngine 安全性
// ──────────────────────────────────────────

func TestNilEngine(t *testing.T) {
	var eng *booksnapgate.Engine
	ctx := context.Background()
	if _, err := eng.PinSnap(ctx, "U", "A", 0, ""); !errors.Is(err, booksnapgate.ErrNilEngine) {
		t.Errorf("PinSnap: %v", err)
	}
	if _, err := eng.Assemble(ctx, "L"); !errors.Is(err, booksnapgate.ErrNilEngine) {
		t.Errorf("Assemble: %v", err)
	}
	if _, err := eng.RePin(ctx, "L"); !errors.Is(err, booksnapgate.ErrNilEngine) {
		t.Errorf("RePin: %v", err)
	}
	if err := eng.Abort(ctx, "L", ""); !errors.Is(err, booksnapgate.ErrNilEngine) {
		t.Errorf("Abort: %v", err)
	}
	if _, err := eng.GetLease(ctx, "L"); !errors.Is(err, booksnapgate.ErrNilEngine) {
		t.Errorf("GetLease: %v", err)
	}
	if _, err := eng.Health(ctx); !errors.Is(err, booksnapgate.ErrNilEngine) {
		t.Errorf("Health: %v", err)
	}
	if _, err := eng.IdempotentBuild(ctx, "U", "A", "W"); !errors.Is(err, booksnapgate.ErrNilEngine) {
		t.Errorf("IdempotentBuild: %v", err)
	}
	if _, err := eng.TryBuildWithRepin(ctx, "U", "A", "W", 3); !errors.Is(err, booksnapgate.ErrNilEngine) {
		t.Errorf("TryBuildWithRepin: %v", err)
	}
}

// ──────────────────────────────────────────
// TestLeaseInfoConversions
// ──────────────────────────────────────────

func TestLeaseInfoConversions(t *testing.T) {
	baseTime := time.Date(2021, 11, 22, 10, 0, 0, 0, time.UTC)
	sealedAt := baseTime.Add(5 * time.Minute)
	lease := booksnapgate.BookSnapLease{
		ID:	"L-INFO-001", UserID: "U-001", Asset: "USDT",
		State:	booksnapgate.LeaseSealed, SnapGen: 3,
		ExpiresAt:	baseTime.Add(10 * time.Minute),
		BookSnaps: []booksnapgate.BookSnap{
			{Book: "funding", SnapID: "S1"},
			{Book: "trading", SnapID: "S2"},
		},
		SealedAt:	&sealedAt,
		SealedResult:	&booksnapgate.MultiConserve{Conserved: true},
	}
	info := booksnapgate.ToLeaseInfo(lease)
	if info.BookCount != 2 {
		t.Errorf("book count = %d", info.BookCount)
	}
	if !info.Sealed {
		t.Error("expected sealed")
	}
	if !info.HasResult {
		t.Error("expected has result")
	}
	list := booksnapgate.ToLeaseInfoList([]booksnapgate.BookSnapLease{lease})
	if len(list) != 1 {
		t.Errorf("list len = %d", len(list))
	}
}

// ──────────────────────────────────────────
// TestIsRetryable
// ──────────────────────────────────────────

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err	error
		want	bool
	}{
		{booksnapgate.ErrDriftDetected, true},
		{booksnapgate.ErrRePinRequired, true},
		{booksnapgate.ErrAssembleAborted, true},
		{booksnapgate.ErrLeaseExpired, false},
		{booksnapgate.ErrUserInvalid, false},
		{booksnapgate.ErrNilEngine, false},
		{nil, false},
	}
	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			if got := booksnapgate.IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable = %v, want %v", got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────

func allBillsFlat(m map[string][]booksnapgate.BillEntry,) []booksnapgate.BillEntry {
	var out []booksnapgate.BillEntry
	for _, bills := range m {
		out = append(out, bills...)
	}
	return out
}
