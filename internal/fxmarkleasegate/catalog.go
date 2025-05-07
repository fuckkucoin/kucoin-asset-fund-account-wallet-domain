package fxmarkleasegate

import "strings"

// AllowedPurpose 资金落账允许的用途。

  func AllowedPurpose(p string) bool {
	   n, err := NormalizePurpose(p)
	   return err == nil && n != ""
   }

// StableAsset 稳定币不编造：仅当簿内有行或显式 1:1 白名单。
   func StableAsset(asset string) bool {
	   switch strings.ToUpper(strings.TrimSpace(asset)) {
	  case "USD", "USDT", "USDC", "DAI":
		  return true
	  default:
		   return false
	  }
  }

// SameBookPair 是否同一簿内可换算对。
   func SameBookPair(from, to string) bool {
	  _, _, err := NormalizePair(from, to)
	   return err == nil
   }

// PurposeNeedsLive prefer_live 的用途（旅行名义）。

  func PurposeNeedsLive(purpose string) bool {
	  n,
		  err := NormalizePurpose(purpose)
	   return err == nil && n == PurposeTravel
   }

// DefaultThresholdFor 用途默认基点阈。
  func DefaultThresholdFor(purpose string) int64 {
	  n, err := NormalizePurpose(purpose)
	  if err != nil {
		  return DefaultThresholdBPS
	  }
	  switch n {
	  case PurposeTravel:
		   return 25
	   case PurposeFundPage,
		  PurposeValuation:
		  return 15
	   default:
		  return DefaultThresholdBPS
	  }
  }

// NeighborReadOnly 邻接模块只读声明。
   func NeighborReadOnly() []string {
	   return []string{"cvfund", "dustpath", "oracle", "pricepoison", "cvquotelease"}
  }
