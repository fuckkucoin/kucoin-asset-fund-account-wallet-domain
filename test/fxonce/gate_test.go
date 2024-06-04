package fxonce_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/fxonce"
)

// ─── PolicyRegistry ──────────────────────────────────────────────────

func TestPolicyRegistry_DefaultFundKindGateRequired(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	for _, fk := range fxonce.AllFundKinds() {
		p := r.Get(string(fk))
		if !p.GateRequired {
			t.Errorf("PolicyRegistry: fund kind %q should have GateRequired=true by default", fk)
		}
		if p.Blocked {
			t.Errorf("PolicyRegistry: fund kind %q should not be Blocked by default", fk)
		}
	}
}

func TestPolicyRegistry_DefaultNonFundKindBypass(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	nonFund := []string{"audit.log", "notify.email", "deposit.credited"}
	for _, kind := range nonFund {
		p := r.Get(kind)
		if p.GateRequired {
			t.Errorf("PolicyRegistry: non-fund kind %q should have GateRequired=false by default", kind)
		}
	}
}

func TestPolicyRegistry_ExplicitOverride(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	// 将非资金 kind 纳入效果闸
	_ = r.Set(&fxonce.KindPolicy{Kind: "audit.log", GateRequired: true, Blocked: false, MaxRetry: 3})
	p := r.Get("audit.log")
	if !p.GateRequired {
		t.Error("explicit override: audit.log should be GateRequired=true")
	}
}

func TestPolicyRegistry_BlockedKind(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	_ = r.Set(&fxonce.KindPolicy{Kind: string(fxonce.KindCreditIntent), GateRequired: true, Blocked: true})
	if err := r.Check(string(fxonce.KindCreditIntent)); err != fxonce.ErrKindBlocked {
		t.Errorf("blocked kind: want ErrKindBlocked, got %v", err)
	}
}

func TestPolicyRegistry_CheckNonBlocked(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	for _, fk := range fxonce.AllFundKinds() {
		if err := r.Check(string(fk)); err != nil {
			t.Errorf("PolicyRegistry.Check(%q) unexpected error: %v", fk, err)
		}
	}
}

func TestPolicyRegistry_MaxRetry(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	_ = r.Set(&fxonce.KindPolicy{Kind: "credit_intent", GateRequired: true, MaxRetry: 10})
	if mr := r.MaxRetry("credit_intent"); mr != 10 {
		t.Errorf("MaxRetry: want 10, got %d", mr)
	}
}

func TestPolicyRegistry_MaxRetry_DefaultFallback(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	r.SetDefaultMaxRetry(7)
	// 未显式配置 max_retry 的 kind 应使用默认值
	mr := r.MaxRetry("some.new.kind")
	if mr != 7 {
		t.Errorf("MaxRetry fallback: want 7, got %d", mr)
	}
}

func TestPolicyRegistry_List_NotEmpty(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	list := r.List()
	if len(list) == 0 {
		t.Error("List: expected non-empty policy list after NewPolicyRegistry")
	}
}

// ─── KindGate ────────────────────────────────────────────────────────

func TestKindGate_FundKind_MustGate(t *testing.T) {
	gate := fxonce.NewKindGate(fxonce.NewPolicyRegistry())
	for _, fk := range fxonce.AllFundKinds() {
		needsGate, err := gate.MustRoute(string(fk))
		if err != nil {
			t.Errorf("KindGate.MustRoute(%q) unexpected error: %v", fk, err)
		}
		if !needsGate {
			t.Errorf("KindGate.MustRoute(%q) = false, want true for fund kind", fk)
		}
	}
}

func TestKindGate_NonFundKind_Bypass(t *testing.T) {
	gate := fxonce.NewKindGate(fxonce.NewPolicyRegistry())
	nonFund := []string{"notify.email", "audit.log", "market.update"}
	for _, k := range nonFund {
		needsGate, err := gate.MustRoute(k)
		if err != nil {
			t.Errorf("KindGate.MustRoute(%q) unexpected error: %v", k, err)
		}
		if needsGate {
			t.Errorf("KindGate.MustRoute(%q) = true, want false (bypass) for non-fund kind", k)
		}
	}
}

func TestKindGate_BlockedKind_ReturnsError(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	_ = r.Set(&fxonce.KindPolicy{Kind: "freeze_intent", GateRequired: true, Blocked: true})
	gate := fxonce.NewKindGate(r)
	_, err := gate.MustRoute("freeze_intent")
	if err != fxonce.ErrKindBlocked {
		t.Errorf("blocked kind: want ErrKindBlocked, got %v", err)
	}
}

func TestKindGate_Decide_PolicySource(t *testing.T) {
	r := fxonce.NewPolicyRegistry()
	gate := fxonce.NewKindGate(r)
	// 预注册的资金 kind 应是 explicit
	d := gate.Decide("credit_intent")
	if d.PolicySource != "explicit" {
		t.Errorf("credit_intent policy source: want explicit, got %q", d.PolicySource)
	}
	// 未注册的 kind 应是 default
	d2 := gate.Decide("some.unknown.kind")
	if d2.PolicySource != "default" {
		t.Errorf("unknown kind policy source: want default, got %q", d2.PolicySource)
	}
}

func TestKindGate_HTTPRateLimit_NotFundKind(t *testing.T) {
	// HTTP 限速码不应被误认为资金 kind
	if fxonce.IsFundKind(string(fxonce.RejectHTTPRateLimit)) {
		t.Error("http_rate_limit must NOT be a fund kind")
	}
	gate := fxonce.NewKindGate(fxonce.NewPolicyRegistry())
	// http_rate_limit 不是有效的 kind — 旁路处理（非资金）
	needsGate, err := gate.MustRoute(string(fxonce.RejectHTTPRateLimit))
	if err != nil {
		t.Errorf("rate_limit kind: unexpected error: %v", err)
	}
	if needsGate {
		t.Error("http_rate_limit must NOT require gate (not a fund kind)")
	}
}

// ─── FundEffectGateway ───────────────────────────────────────────────

func TestFundEffectGateway_FundKind_Applied(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	gw := fxonce.NewFundEffectGateway(store, nil)
	ctx := context.Background()
	result, bypassed, err := gw.RouteApply(ctx, fxonce.MarkRequest{
		BusinessKey:	"dep-gw-001",
		Kind:		fxonce.KindCreditIntent,
		ExpectedGen:	1,
	})
	if err != nil {
		t.Fatalf("RouteApply: %v", err)
	}
	if bypassed {
		t.Error("fund kind should not be bypassed")
	}
	if result.Rejected {
		t.Errorf("fund kind apply rejected: %v", result.RejectCode)
	}
}

func TestFundEffectGateway_NonFundKind_Bypass(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	gw := fxonce.NewFundEffectGateway(store, nil)
	ctx := context.Background()
	result, bypassed, err := gw.RouteApply(ctx, fxonce.MarkRequest{
		BusinessKey:	"audit-001",
		Kind:		"audit.log",
		ExpectedGen:	1,
	})
	if err != nil {
		t.Fatalf("RouteApply non-fund: %v", err)
	}
	if !bypassed {
		t.Error("non-fund kind should be bypassed")
	}
	if result.Rejected {
		t.Error("bypassed kind should not be rejected")
	}
}

func TestFundEffectGateway_BlockedKind_Rejected(t *testing.T) {
	t.Parallel()
	r := fxonce.NewPolicyRegistry()
	_ = r.Set(&fxonce.KindPolicy{Kind: "credit_intent", GateRequired: true, Blocked: true})
	store := fxonce.NewMemStore()
	gw := fxonce.NewFundEffectGateway(store, r)
	ctx := context.Background()
	result, bypassed, err := gw.RouteApply(ctx, fxonce.MarkRequest{
		BusinessKey:	"dep-gw-002",
		Kind:		fxonce.KindCreditIntent,
		ExpectedGen:	1,
	})
	if err != fxonce.ErrKindBlocked {
		t.Errorf("blocked kind: want ErrKindBlocked, got %v", err)
	}
	if !result.Rejected {
		t.Error("blocked kind should be rejected")
	}
	if bypassed {
		t.Error("blocked kind should not be bypassed")
	}
}
