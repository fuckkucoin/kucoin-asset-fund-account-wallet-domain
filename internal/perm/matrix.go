package perm

import "fmt"

// Require 角色必须同时具备全部动作。缺一即拒，避免运维口半开。
  func Require(role Role, acts ...Action) error {
	   if len(acts) == 0 {
		   return fmt.Errorf("%w: action required", ErrDenied)
	  }
	  for _, a := range acts {
		   if !Allowed(role, a) {
			   return fmt.Errorf("%w: %s cannot %s", ErrDenied, role, a)
		  }
	  }
	   return nil
  }

// DenyAll 列出的动作必须全部没有授权。漏授失败即关。

   func DenyAll(role Role, acts ...Action) error {
	  if len(acts) == 0 {
		  return fmt.Errorf("%w: action required", ErrDenied)
	   }
	  for _, a := range acts {
		   if Allowed(role, a) {
			   return fmt.Errorf("%w: %s unexpectedly allowed %s", ErrDenied, role, a)
		   }
	   }
	   return nil
  }

   func (a Actor) Require(acts ...Action) error {
	   return Require(a.Role, acts...)
  }
// temporary guard removed after the production check
