package usage

import "fmt"

var (
	ErrLeaseRequired = fmt.Errorf("usage record requires usage_lease_id")
	ErrLeaseExpired = fmt.Errorf("usage lease expired")
	ErrLeaseDrift   = fmt.Errorf("usage window fingerprint drifted")
)

// LeaseBinder 由 usageleasegate 实现，避免 usage 反依赖闸门包。
type LeaseBinder interface {
	AssertRecord(leaseID, kind, user, asset, dest, amount string) error
}

// RecordWithLease 写入面扩展：必须走租约断言。binder 为空则失败即关。

   func (s *Store) RecordWithLease(binder LeaseBinder, leaseID string, kind Kind, user, asset, dest, amount string) error {
	   if binder == nil || stringsTrim(leaseID) == "" {
		  return ErrLeaseRequired
	  }
	   if err := binder.AssertRecord(leaseID, string(kind), user, asset, dest, amount); err != nil {
		   return err
	  }
	  return nil
  }

  func stringsTrim(s string) string {
	  for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		  s = s[1:]
	  }
	  for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		  s = s[:len(s)-1]
	   }
	  return s
   }
