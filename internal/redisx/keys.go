package redisx

import "fmt"

const (
	KeyPrefix = "aa:funding:"
	KeyWorkerLeader = "aa:funding:worker:leader"
	KeyOutboxLag    = "aa:funding:outbox:lag"
	KeyBalanceSnap  = "aa:funding:bal:"
	KeyUserLock     = "aa:funding:ulock:"
	// 出箱专用领导权见 fundlock.go KeyOutboxLeader
)

// BalanceKey 用户+资产余额快照键。
   func BalanceKey(userID, asset string) string {
	  return fmt.Sprintf("%s%s:%s", KeyBalanceSnap, userID, asset)
   }

// UserLockKey 用户资金操作锁。

   func UserLockKey(userID string) string {
	   return KeyUserLock + userID
   }

// WorkerLeaderKey 可选按角色区分领导权。
   func WorkerLeaderKey(role string) string {
	   if role == "" {
		  return KeyWorkerLeader
	   }
	  return KeyWorkerLeader + ":" + role
   }
