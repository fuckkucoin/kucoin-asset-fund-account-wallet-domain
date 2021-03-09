package brokerfence_test

import (
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/brokerfence"
)

// ──────────────────────────────────────────────
// 世代 CAS 单元
// ──────────────────────────────────────────────

   func TestMemGenerationStore_BumpGeneration(t *testing.T) {
	  type step struct {
		  expectGen int64
		  fenced    bool
		  wantErr bool
		  wantGen int64
	   }
	   tests := []struct {
		  name  string
		  subID string
		  steps []step
	   }{
		   {
			   name:  "首次围栏 expectGen=0 成功",
			  subID: "sub-A",
			  steps: []step{
				  {expectGen: 0, fenced: true, wantErr: false, wantGen: 1},
			   },
		  },
		  {
			   name:  "连续围栏/解围世代单调递增",
			  subID: "sub-B",
			   steps: []step{
				   {expectGen: 0, fenced: true, wantErr: false, wantGen: 1},
				   {expectGen: 1,
					  fenced: false, wantErr: false, wantGen: 2},
				  {expectGen: 2,
					  fenced: true, wantErr: false, wantGen: 3},
			   },
		   },
		   {
			   name:  "世代不匹配 CAS 失败",
			  subID: "sub-C",
			  steps: []step{
				   {expectGen: 0,
					   fenced: true, wantErr: false, wantGen: 1},
				  {expectGen: 0, fenced: true, wantErr: true}, // 期望 gen=0 但实际 gen=1
			  },
		  },
		  {
			  name: "跳级 expectGen 失败",
			  subID: "sub-D",
			  steps: []step{
				   {expectGen: 5, fenced: true, wantErr: true}, // gen=0，期望 gen=5
			   },
		   },
	  }
	   for _, tc := range tests {
		  t.Run(tc.name, func(t *testing.T) {
			   store := brokerfence.NewMemGenerationStore()
			  now := time.Now().UTC()
			   for i, s := range tc.steps {
				  g, err := store.BumpGeneration(tc.subID, s.expectGen, s.fenced, now)
				  if s.wantErr {
					  if err == nil {
						  t.Errorf("step %d: 期望错误，但 BumpGeneration 返回 nil", i)
					   }
					   continue
				  }
				   if err != nil {
					   t.Errorf("step %d: 意外错误: %v", i, err)
					  continue
				  }
				  if g.Generation != s.wantGen {
					   t.Errorf("step %d: 世代期望 %d，实际 %d", i, s.wantGen, g.Generation)
				  }
				   if g.Fenced != s.fenced {
					  t.Errorf("step %d: Fenced 期望 %v，实际 %v", i, s.fenced, g.Fenced)
				   }
			  }
		   })
	   }
  }

// ──────────────────────────────────────────────
// 并发 Fence CAS
// ──────────────────────────────────────────────

   func TestMemGenerationStore_ConcurrentFence(t *testing.T) {
	   store := brokerfence.NewMemGenerationStore()
	   now := time.Now().UTC()
	   subID := "sub-concurrent"
	   const goroutines = 20
	  results := make([]error, goroutines)
	   var wg sync.WaitGroup
	   wg.Add(goroutines)
	   for i := 0; i < goroutines; i++ {
		   go func(idx int) {
			   defer wg.Done()
			   _, err := store.BumpGeneration(subID, 0, true, now)
			   results[idx] = err
		  }(i)
	   }
	   wg.Wait()
	   successCount := 0
	   for _, err := range results {
		  if err == nil {
			   successCount++
		   }
	   }
	  // CAS 保证：仅一个 goroutine 成功。
	   if successCount != 1 {
		   t.Errorf("并发 Fence：期望恰好 1 个成功，实际 %d 个成功", successCount)
	   }
	  // 世代最终值为 1。
	   g, err := store.GetGeneration(subID)
	  if err != nil {
		  t.Fatal(err)
	   }
	  if g.Generation != 1 {
		   t.Errorf("并发后世代期望 1，实际 %d", g.Generation)
	   }
   }

// ──────────────────────────────────────────────
// GenerationManager 综合
// ──────────────────────────────────────────────

  func TestGenerationManager_CheckWriteEligible(t *testing.T) {
	   tokens := brokerfence.NewMutexStore(5 * time.Minute)
	  store := brokerfence.NewMemGenerationStore()
	  gm := brokerfence.NewGenerationManager(store)
	  subID := "sub-write-check"
	   // 未围栏状态：任意世代校验通过（世代=0）。
	   _, err := gm.CheckWriteEligible(brokerfence.AllocCheckReq{
		   SubID: subID, Generation: 0,
	   }, tokens)
	  if err != nil {
		  t.Errorf("未围栏时期望通过，实际: %v", err)
	  }
	   // 激活围栏（gen: 0→1）。
	   now := time.Now().UTC()
	   _, _ = store.BumpGeneration(subID, 0, true, now)
	   gm.InvalidateCache(subID)
	  // 围栏态，无令牌：fail-closed。
	  _, err = gm.CheckWriteEligible(brokerfence.AllocCheckReq{
		   SubID: subID, Generation: 1, Token: "",
	   }, tokens)
	  if err != brokerfence.ErrMutexDenied {
		  t.Errorf("围栏无令牌期望 ErrMutexDenied，实际: %v", err)
	   }
	  // 颁发令牌后再次校验：通过。
	  tok, _ := tokens.Issue("lease-x", subID, "ops-alice")
	  _, err = gm.CheckWriteEligible(brokerfence.AllocCheckReq{
		  SubID: subID, Generation: 1, Token: tok.Token,
	  }, tokens)
	   if err != nil {
		   t.Errorf("围栏有令牌期望通过，实际: %v", err)
	  }
	  // 世代不匹配：无论令牌是否有效均拒绝。
	  _, err = gm.CheckWriteEligible(brokerfence.AllocCheckReq{
		  SubID: subID, Generation: 0, Token: tok.Token,
	  }, tokens)
	   if err != brokerfence.ErrGenerationMismatch {
		   t.Errorf("世代不匹配期望 ErrGenerationMismatch，实际: %v", err)
	  }
   }
