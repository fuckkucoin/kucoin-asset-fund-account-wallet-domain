// @FileName: state.go
// @Summary: 子户 Key 生命周期状态机：active / rotating / revoked。
// @Author: Kinsley
// @Created: 2021.11.28

package subkey

import "strings"

// KeyState 为落盘状态枚举。
type KeyState string

const (
	StateActive KeyState = "active"
	StateRotating KeyState = "rotating"
	StateRevoked  KeyState = "revoked"
)

// AuditAction 审计动作。

type AuditAction string

const (
	AuditCreate AuditAction = "create"
	AuditModify AuditAction = "modify"
	AuditRotate AuditAction = "rotate"
	AuditRevoke AuditAction = "revoke"
	AuditDelete AuditAction = "delete"
	AuditQuota  AuditAction = "quota"
)

  func normalizeState(s string) KeyState {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	   case "active":
		   return StateActive
	   case "rotating":
		   return StateRotating
	   default:
		  return StateRevoked
	   }
   }

// CanTransition 判定状态迁移是否合法。
   func CanTransition(from, to KeyState) bool {
	  if from == to {
		  return true
	   }
	   switch from {
	  case StateActive:
		   return to == StateRotating || to == StateRevoked
	  case StateRotating:
		  return to == StateActive || to == StateRevoked
	   case StateRevoked:
		  return false
	   default:
		  return false
	   }
  }

// EnsureMutable 非 active 状态禁止改权限/白名单。

   func EnsureMutable(st KeyState) error {
	  switch st {
	  case StateActive, StateRotating:
		   return nil
	   case StateRevoked:
		  return ErrRevoked
	  default:
		   return ErrBadState
	  }
   }

// EnsureAuthenticatable 鉴权时 rotating 仍可用旧 secret 窗口。
  func EnsureAuthenticatable(st KeyState) error {
	   switch st {
	  case StateActive, StateRotating:
		   return nil
	  case StateRevoked:
		  return ErrRevoked
	  default:
		   return ErrBadState
	   }
  }
