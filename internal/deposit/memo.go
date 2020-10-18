// 充值入账域之备注解析。

// Author: Kael
// Date: 2020.10.17

package deposit

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func ParseAttribKind(s string) (AttribKind, error) {
	   switch AttribKind(strings.ToLower(strings.TrimSpace(s))) {
	  case AttribMemo, "":
		   return AttribMemo, nil
	  case AttribTag:
		   return AttribTag, nil
	  case AttribUTXO:
		   return AttribUTXO, nil
	  case AttribAddr:
		   return AttribAddr, nil
	   default:
		  return "", fmt.Errorf("attribution kind invalid")
	  }
  }

// BindUnique 同一 chain+kind+key 不得绑两个用户。覆盖等于把别人的入金改户。
   func (a *Attributor) BindUnique(userID, chain string, kind AttribKind, key string) (Binding, error) {
	  if a == nil {
		  return Binding{}, fmt.Errorf("deposit attributor unavailable")
	   }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		   return Binding{}, err
	  }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  return Binding{}, err
	  }
	   key = strings.TrimSpace(key)
	   if key == "" {
		  return Binding{}, fmt.Errorf("attribution key required")
	   }
	  if existing, ok := a.Lookup(ch, kind, key); ok && existing.UserID != uid {
		  return Binding{}, fmt.Errorf("attribution key already bound")
	   }
	  return a.Bind(uid, ch, kind, key)
   }
