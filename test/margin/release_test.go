package margin_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/margin"
)

func TestReleaseForFillTable(t *testing.T) {
	cases := []struct {
		name	string
		fill	string
		left	string
		want	string
		wantErr	bool
	}{
		{name: "小于剩余", fill: "20", left: "50", want: "20"},
		{name: "等于剩余", fill: "50", left: "50", want: "50"},
		{name: "大于剩余封顶", fill: "80", left: "50", want: "50"},
		{name: "零成交", fill: "0", left: "50", want: "0"},
		{name: "负成交失败", fill: "-1", left: "50", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := margin.ReleaseForFill(tc.fill, tc.left)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %s err=%v want %s", got, err, tc.want)
			}
		})
	}
}

func TestAssertConserve(t *testing.T) {
	if err := margin.AssertConserve("500", "200", "300"); err != nil {
		t.Fatal(err)
	}
	if err := margin.AssertConserve("500", "200", "200"); err == nil {
		t.Fatal("broken conserve must fail")
	}
}
