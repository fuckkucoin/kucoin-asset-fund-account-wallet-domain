package keyrotatefence

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/keysrc"
)

  func refuseProdMaterial(authKind, profile, source, keyPath string) error {
	  if err := keysrc.RefuseLocalHMAC(authKind, profile, source); err != nil {
		  return ErrLocalHMAC
	  }
	  if err := keysrc.RefuseSource(authKind, profile, source); err != nil {
		   if err == keysrc.ErrLocalHMAC {
			  return ErrLocalHMAC
		   }
		   return ErrProdFallback
	   }
	  if err := keysrc.RefuseDemoKey(authKind, profile, keyPath); err != nil {
		  return ErrDemoKey
	  }
	  if keysrc.Production(authKind, profile) && strings.TrimSpace(source) == "" {
		   return ErrProdFallback
	   }
	  return nil
  }
