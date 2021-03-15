package brokerfence_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/brokerfence"
)

   func TestMutexStore_IssueAndValidate(t *testing.T) {
	  ms := brokerfence.NewMutexStore(10 * time.Minute)
	  // 颁发令牌。
	  tok1, err := ms.Issue("lease-1", "sub-1", "ops-alice")
	  if err != nil {
		  t.Fatalf("Issue 失败: %v", err)
	   }
	   // 有效令牌校验通过。
	  now := time.Now().UTC()
	   if err := ms.ValidateToken("sub-1", tok1.Token, now); err != nil {
		   t.Errorf("有效令牌期望通过，实际: %v", err)
	   }
	   // 错误令牌值拒绝。
	  if err := ms.ValidateToken("sub-1", "wrong-token", now); err != brokerfence.ErrMutexDenied {
		   t.Errorf("错误令牌期望 ErrMutexDenied，实际: %v", err)
	  }
	  // 后颁令牌覆盖。
	   tok2, _ := ms.Issue("lease-1", "sub-1", "ops-bob")
	   if ms.ValidateToken("sub-1", tok1.Token, now) == nil {
		   t.Error("旧令牌应被新令牌覆盖，期望失败")
	  }
	  if err := ms.ValidateToken("sub-1", tok2.Token, now); err != nil {
		   t.Errorf("新令牌期望通过，实际: %v", err)
	   }
   }

  func TestMutexStore_Revoke(t *testing.T) {
	  ms := brokerfence.NewMutexStore(10 * time.Minute)
	  tok, _ := ms.Issue("lease-2", "sub-2", "ops-x")
	   ms.Revoke("sub-2")
	  now := time.Now().UTC()
	   if err := ms.ValidateToken("sub-2", tok.Token, now); err != brokerfence.ErrMutexDenied {
		  t.Errorf("撤销后期望 ErrMutexDenied，实际: %v", err)
	   }
   }

   func TestMutexStore_PurgeExpired(t *testing.T) {
	  ms := brokerfence.NewMutexStore(time.Millisecond)
	  ms.Issue("lease-3", "sub-3a", "ops-a")
	   ms.Issue("lease-3", "sub-3b", "ops-b")
	   // 手动设置过去时间触发过期（修改内部 clock）。
	  // 由于 clock 未暴露，用等待替代。
	  time.Sleep(5 * time.Millisecond)
	   removed := ms.PurgeExpired()
	   if removed != 2 {
		   t.Errorf("期望清理 2 个过期令牌，实际 %d", removed)
	   }
  }

   func TestMutexStore_MissingSubID(t *testing.T) {
	   ms := brokerfence.NewMutexStore(10 * time.Minute)
	   now := time.Now().UTC()
	  if err := ms.ValidateToken("nonexistent-sub", "any-token", now); err != brokerfence.ErrMutexDenied {
		  t.Errorf("不存在子账期望 ErrMutexDenied，实际: %v", err)
	   }
   }

  func TestMutexStore_InvalidInput(t *testing.T) {
	   ms := brokerfence.NewMutexStore(10 * time.Minute)
	   // leaseID 空。
	   if _, err := ms.Issue("", "sub-x", "actor"); err != brokerfence.ErrInput {
		   t.Errorf("leaseID 空期望 ErrInput，实际: %v", err)
	  }
	   // subID 空。
	  if _, err := ms.Issue("lease-x", "", "actor"); err != brokerfence.ErrInput {
		  t.Errorf("subID 空期望 ErrInput，实际: %v", err)
	  }
	  // actor 空。
	   if _, err := ms.Issue("lease-x", "sub-x", ""); err != brokerfence.ErrInput {
		  t.Errorf("actor 空期望 ErrInput，实际: %v", err)
	  }
   }
