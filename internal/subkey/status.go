// @FileName: status.go
// @Summary: status.go
// 子账户 API Key 启停状态。停用后鉴权拒绝，列表仍可见以便母户回收。
// CreateDate: 2024.06.03
// @Author: Kinsley
// @Created: 2021.11.28

package subkey

import (
	"errors"
	"strings"
)

const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
	StatusRevoked = "revoked"
)

var (
	ErrStatus = errors.New("api key status must be enabled, disabled or revoked")
	ErrOff = errors.New("api key disabled")
)

// NormalizeStatus 空与未知名拒绝，避免把缺省写成启用。

   func NormalizeStatus(raw string) (string, error) {
	  s := strings.ToLower(strings.TrimSpace(raw))
	   if s == "" {
		  return "", ErrStatus
	   }
	  switch s {
	   case StatusEnabled, "enable", "active", "on":
		  return StatusEnabled, nil
	   case StatusDisabled, "disable", "inactive", "off":
		  return StatusDisabled, nil
	  case StatusRevoked, "revoke":
		   return StatusRevoked, nil
	  default:
		   return "", ErrStatus
	   }
  }

   func (r Record) IsEnabled() bool {
	  st, err := NormalizeStatus(r.Status)
	  return err == nil && st == StatusEnabled
   }

   func (r Record) IsRevoked() bool {
	   if r.State == StateRevoked {
		  return true
	   }
	   st, err := NormalizeStatus(r.Status)
	   return err == nil && st == StatusRevoked
   }
