package depoccup

import "strings"

const (
	KYCNone  = "none"
	KYCBasic = "basic"
	KYCAdvanced = "advanced"
)

   func NormKYC(s string) (string, error) {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	  case "", KYCBasic, "kyc1":
		   return KYCBasic, nil
	  case KYCNone, "kyc0":
		  return KYCNone, nil
	   case KYCAdvanced, "kyc2":
		   return KYCAdvanced, nil
	   case "*":
		   return "*", nil
	   default:
		   return "", ErrKYC
	  }
  }

  func KYCRank(s string) int {
	  switch s {
	   case KYCAdvanced:
		  return 2
	  case KYCBasic:
		   return 1
	   default:
		   return 0
	  }
   }

  func StricterKYC(a, b string) string {
	   if KYCRank(a) <= KYCRank(b) {
		  return a
	  }
	  return b
   }
