package productdedup_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/productdedup"
)

func TestActiveDefault(t *testing.T) {
	if !productdedup.Active(false, true) {
		t.Fatal("expected dedup active when mesh ok")
	}
	if productdedup.Active(true, true) {
		t.Fatal("mem should disable dedup")
	}
}

func TestFallbackMem(t *testing.T) {
	if !productdedup.FallbackAllowed(true) {
		t.Fatal("mem allows fallback")
	}
}
