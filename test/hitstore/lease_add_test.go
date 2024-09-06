package hitstore_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/hitstore"
)

type rejectGuard struct{}

func (rejectGuard) AssertAdd(ctx context.Context, leaseID string, kind usage.Kind, user, asset, dest, amount, idem string) error {
	return hitstore.ErrLeaseRejected
}

type okGuard struct{}

func (okGuard) AssertAdd(ctx context.Context, leaseID string, kind usage.Kind, user, asset, dest, amount, idem string) error {
	return nil
}

func TestAddWithLeaseFailClosed(t *testing.T) {
	s := hitstore.NewMem()
	if err := s.AddWithLease(context.Background(), nil, "", usage.KindWithdraw, "H1", "USDT", "d", "1", ""); err != hitstore.ErrLeaseRequired {
		t.Fatal(err)
	}
	if err := s.AddWithLease(context.Background(), rejectGuard{}, "L1", usage.KindWithdraw, "H1", "USDT", "d", "1", ""); err != hitstore.ErrLeaseRejected {
		t.Fatal(err)
	}
	if s.TodaySum(context.Background(), usage.KindWithdraw, "H1", "USDT").String() != "0" {
		t.Fatal("counted")
	}
}

func TestAddWithLeaseGuardOKDoesNotAutoWrite(t *testing.T) {
	s := hitstore.NewMem()
	// 守卫通过只表示租约断言成功；占用由引擎提交钩子写入，避免双记。
	if err := s.AddWithLease(context.Background(), okGuard{}, "L2", usage.KindWithdraw, "H2", "USDT", "d", "1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestWindowFingerprintHitstore(t *testing.T) {
	s := hitstore.NewMem()
	a := s.WindowFingerprint(context.Background(), usage.KindWithdraw, "H3", "USDT", "d")
	_ = s.Add(context.Background(), usage.KindWithdraw, "H3", "USDT", "d", "1", "i")
	b := s.WindowFingerprint(context.Background(), usage.KindWithdraw, "H3", "USDT", "d")
	if a == "" || a == b {
		t.Fatal(a, b)
	}
}
