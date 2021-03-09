package brokerfence_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/brokerfence"
)

// stubBookReader 桩：始终返回预设余额合计。
type stubBookReader struct {
	totals map[string]string // key=masterID+"|"+asset
}

   func (r *stubBookReader) TotalOf(masterID, asset string) (string, error) {
	   v, ok := r.totals[masterID+"|"+asset]
	   if !ok {
		  return "0", nil
	   }
	   return v, nil
  }

  func TestConserveEngine_SampleAndDrift(t *testing.T) {
	  tests := []struct {
		  name        string
		   beforeTotal string
		  afterTotal  string
		  wantDrifted bool
		   wantDelta string
	  }{
		   {
			  name: "无漂移：before=after=1000",
			   beforeTotal: "1000",
			   afterTotal:  "1000",
			  wantDrifted: false,
			  wantDelta:   "0",
		  },
		   {
			   name: "正向漂移：after>before",
			  beforeTotal: "1000",
			   afterTotal:  "1050",
			   wantDrifted: true,
			  wantDelta:   "50",
		  },
		   {
			   name:        "负向漂移：after<before",
			   beforeTotal: "1000",
			  afterTotal:  "980",
			   wantDrifted: true,
			   wantDelta: "-20",
		   },
		   {
			   name:        "零余额无漂移",
			  beforeTotal: "0",
			   afterTotal: "0",
			   wantDrifted: false,
			   wantDelta: "0",
		  },
	   }
	   for _, tc := range tests {
		   t.Run(tc.name, func(t *testing.T) {
			  leaseID := "lease-" + tc.name
			   masterID := "master-001"
			   asset := "USDT"
			   reader := &stubBookReader{
				   totals: map[string]string{
					  masterID + "|" + asset: tc.beforeTotal,
				  },
			   }
			  store := brokerfence.NewMemConserveStore()
			   ce := brokerfence.NewConserveEngine(store, reader)
			  // 采样 before。
			   _, err := ce.SampleBefore(leaseID, "sub-001", masterID, asset)
			  if err != nil {
				   t.Fatalf("SampleBefore 失败: %v", err)
			  }
			  // 切换余额到 after。
			  reader.totals[masterID+"|"+asset] = tc.afterTotal
			   // 采样 after。
			  proof,
				   err := ce.SampleAfter(leaseID)
			  if err != nil {
				   t.Fatalf("SampleAfter 失败: %v", err)
			   }
			   if proof.IsDrifted != tc.wantDrifted {
				   t.Errorf("IsDrifted 期望 %v，实际 %v", tc.wantDrifted, proof.IsDrifted)
			  }
			   if proof.Delta != tc.wantDelta {
				  t.Errorf("Delta 期望 %s，实际 %s", tc.wantDelta, proof.Delta)
			   }
			   // 验证 CheckDrift 与漂移标记一致。
			   err = ce.CheckDrift(leaseID)
			   if tc.wantDrifted {
				   if err != brokerfence.ErrConserveDrift {
					  t.Errorf("漂移时 CheckDrift 期望 ErrConserveDrift，实际 %v", err)
				  }
				  // 清除漂移后应通过。
				  ce.ClearDrift(leaseID)
				   if err := ce.CheckDrift(leaseID); err != nil {
					  t.Errorf("清除漂移后 CheckDrift 期望 nil，实际 %v", err)
				  }
			   } else {
				  if err != nil {
					  t.Errorf("无漂移时 CheckDrift 期望 nil，实际 %v", err)
				   }
			  }
		   })
	  }
   }

   func TestConserveEngine_LatestProof(t *testing.T) {
	  reader := &stubBookReader{totals: map[string]string{"master-X|USDT": "500"}}
	  store := brokerfence.NewMemConserveStore()
	   ce := brokerfence.NewConserveEngine(store, reader)
	  // 同一 lease 多次采样：LatestProof 返回最新。
	  _, err := ce.SampleBefore("lease-multi", "sub-X", "master-X", "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	   reader.totals["master-X|USDT"] = "600"
	   p1, _ := ce.SampleAfter("lease-multi")
	   reader.totals["master-X|USDT"] = "700"
	   p2, _ := ce.SampleAfter("lease-multi")
	  latest, err := ce.LatestProof("lease-multi")
	  if err != nil {
		  t.Fatal(err)
	   }
	  if latest.AfterTotal != p2.AfterTotal {
		  t.Errorf("LatestProof 期望最新 after_total=%s，实际 %s", p2.AfterTotal, latest.AfterTotal)
	   }
	  _ = p1
  }
