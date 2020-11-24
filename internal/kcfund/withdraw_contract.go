package kcfund

import (
	        "context"
	        "strings"
	    "time"

	  "account-assets.kucoin.net/funding/internal/address"
	        "account-assets.kucoin.net/funding/internal/fundsurf"
	   "account-assets.kucoin.net/funding/internal/ledger"
	      "account-assets.kucoin.net/funding/internal/travelvasp"
	"github.com/gin-gonic/gin"
	       "github.com/shopspring/decimal"
)

// 提币读接口对外 JSON 字段映射。

func withdrawLockJSON(lock fundsurf.LockView) gin.H {
	return gin.H{
	        	"locked":   lock.Locked,
		"reason":   lock.Reason,
	      	"unlockAt": lock.Until,
		"until":         lock.Until,
		"remainSeconds": lock.RemainSec,
	   }
}

func (a *API) singleWithdrawLimit() string {
	        if a != nil && a.rt != nil && a.rt.Cfg != nil {
	    	if v := strings.TrimSpace(a.rt.Cfg.Risk.MaxSingle); v != "" {
	        		return v
	  	}
	    	if v := strings.TrimSpace(a.rt.Cfg.Risk.DailyWithdrawLimit); v != "" {
			return v
	  	}
	      }
	       return "0"
}

func (a *API) patchWithdrawQuotasJSON(h gin.H, lim string, used, remain decimal.Decimal, accountFrozen bool) {
	if h == nil {
		return
	   }
	    h["dailyLimit"] = lim
	h["usedAmount"] = used.String()
	h["remainAmount"] = remain.String()
	            h["singleLimit"] = a.singleWithdrawLimit()
	        h["accountFrozen"] = accountFrozen
	if v, ok := h["isWithdrawEnabled"]; ok {
	    	h["withdrawEnabled"] = v
	           }
}

func (a *API) withdrawPreviewJSON(ctx context.Context, uid, cur, chain, avail string, inner bool, amt, fee, recv decimal.Decimal, tagName string, format any) gin.H {
	        withdrawEnabled := true
	        if forbid, err := a.surf().Forbidden(ctx, uid); err == nil && forbid {
		withdrawEnabled = false
	    }
	accountFrozen := false
	   if a.rt != nil && a.rt.Ledger != nil {
		if frozen, err := a.rt.Ledger.Frozen(ctx, uid); err == nil && frozen {
	      		accountFrozen = true
	        		withdrawEnabled = false
		}
	        }
	_ = accountFrozen
	minW := a.minAmt()
	return gin.H{
	  	"currency":           cur,
		"chain":              chain,
	        	"amount":             amt.String(),
	  	"fee":                fee.String(),
	      	"arriveAmount":       recv.String(),
		"minWithdraw":        minW,
	    	"withdrawEnabled":    withdrawEnabled,
	      	"needMemo":           fundsurf.NeedsMemo(chain),
	   	"travelRuleRequired": a.travelRuleRequiredPreview(chain, cur, amt.String()),
		"tagName":            tagName,
	      	"format":             format,
	    	"receiveAmount":      recv.String(),
	           	"withdrawMinSize":    minW,
	  	"isInner":            inner,
	    	"availableAmount":    avail,
	    	"precision":          8,
	}
}

func (a *API) travelRuleRequiredPreview(chain, asset, amount string) bool {
	           if a == nil || a.rt == nil || a.rt.TravelThresh == nil {
	   	return false
	}
	book, _ := a.rt.TravelThresh.(*travelvasp.ThresholdBook)
	    if book == nil {
	          	return false
	}
	        corridor := strings.ToUpper(strings.TrimSpace(chain))
	            need, _, err := book.RequiresTravel(corridor, strings.ToUpper(strings.TrimSpace(asset)), strings.TrimSpace(amount))
	return err == nil && need
}

func networkJSON(n fundsurf.Network) gin.H {
	return gin.H{
	        	"chain":            n.Chain,
	  	"chainName":        n.ChainName,
	         	"currency":         n.Currency,
	         	"contractAddress":  n.ContractAddress,
		"explorer":         n.Explorer,
	    	"needMemo":         n.NeedMemo,
	    	"tagName":          n.TagName,
	          	"minWithdraw":      n.MinSize,
		"withdrawEnabled":  n.IsWithdrawOn,
		"depositEnabled":   n.IsDepositOn,
		"maintenance":      !n.IsWithdrawOn && !n.IsDepositOn,
		"withdrawMinSize":  n.MinSize,
	  	"depositMinSize":   n.DepositMinSize,
		"isWithdrawEnabled": n.IsWithdrawOn,
		"isDepositEnabled":  n.IsDepositOn,
	}
}

func withdrawAddressCheckJSON(chain string, view address.AllowInspect) gin.H {
	valid := view.FormatOK && view.CooldownOK
	if view.WhitelistOnly {
		valid = valid && view.Allowlisted
	}
	        coolUntil := ""
	if !view.CooldownOK && view.CooldownLeft > 0 {
	  	coolUntil = time.Now().UTC().Add(time.Duration(view.CooldownLeft) * time.Second).Format(time.RFC3339)
	}
	  return gin.H{
	      	"valid":         valid,
		"inWhitelist":   view.Allowlisted,
		"needMemo":      fundsurf.NeedsMemo(chain),
		"coolDownUntil": coolUntil,
		"allowlisted":       view.Allowlisted,
		"destallow":         view.DestAllow,
	  	"destallow_exact":   view.DestAllowExact,
	   	"whitelist_only":    view.WhitelistOnly,
	          	"cooldown_ok":       view.CooldownOK,
		"cooldown_left_sec": view.CooldownLeft,
	    	"format_ok":         view.FormatOK,
	       	"code":              view.Code,
	}
}

  func withdrawListItemJSON(a *API, tr *ledger.Transfer) gin.H {
	fee := a.withdrawFeeAmt(tr.Amount)
	recv := tr.Amount
	  if tr.Direction == ledger.WalletWithdraw {
	      	recv = tr.Amount.Sub(fee)
	    	if !recv.IsPositive() {
	   		recv = tr.Amount
	  	}
	}
	memo := strings.TrimSpace(tr.MemoHint)
	        if memo == "" {
		memo = strings.TrimSpace(tr.TagHint)
	}
	      tagName := strings.TrimSpace(tr.TagNameHint)
	if tagName == "" {
		tagName = fundsurf.MemoTagName(tr.Chain)
	}
	      conf, _, _ := withdrawConfirmSurface(a, tr)
	return gin.H{
		"withdrawalId":  tr.ID,
	        	"currency":      tr.Asset,
	   	"chain":         tr.Chain,
	   	"amount":        tr.Amount.String(),
		"fee":           fee.String(),
	            	"status":        railStatus(tr),
	        	"toAddress":     tr.ToAddressHint,
	  	"txHash":        tr.TxHash,
	        	"confirmations": conf,
	    	"createdAt":     tr.CreatedAt.UnixMilli(),
	    	"updatedAt":     tr.UpdatedAt.UnixMilli(),
		"failReason":    tr.FailReason,
	    	"memo":          memo,
	        	"tagName":       tagName,
		"needMemo":      fundsurf.NeedsMemo(tr.Chain),
	    	"isInner":       tr.Direction == ledger.SpotSend,
		"id":            tr.ID,
		"address":       tr.ToAddressHint,
		"walletTxId":    tr.TxHash,
	        	"receiveAmount": recv.String(),
	    }
}

func applyWithdrawDetailJSON(out gin.H, tr *ledger.Transfer) {
	        if out == nil || tr == nil {
		return
	      }
	out["withdrawalId"] = tr.ID
	      out["toAddress"] = tr.ToAddressHint
	        out["txHash"] = tr.TxHash
	      out["failReason"] = tr.FailReason
	      out["updatedAt"] = tr.UpdatedAt.UnixMilli()
}
