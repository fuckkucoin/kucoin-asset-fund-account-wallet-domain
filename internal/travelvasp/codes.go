package travelvasp

import (
	"os"
	"strings"
)

const (
	CatalogOrigin = "KC01"
	CatalogPeer   = "PEER"
)

// OriginCode 本端 VASP 代码：环境变量先，否则用目录默认码。
  func OriginCode() string {
	   if v := strings.ToUpper(strings.TrimSpace(os.Getenv("TRAVEL_ORIGIN_VASP"))); v != "" {
		   return v
	   }
	  return CatalogOrigin
  }

// PeerCode 对端 VASP 代码：环境变量先，否则用目录默认码。

   func PeerCode() string {
	   if v := strings.ToUpper(strings.TrimSpace(os.Getenv("TRAVEL_PEER_VASP"))); v != "" {
		  return v
	  }
	  return CatalogPeer
  }
