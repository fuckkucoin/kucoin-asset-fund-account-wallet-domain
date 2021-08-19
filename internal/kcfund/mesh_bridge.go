// 资金闸顺序（链上出金）在 rails.withdrawV3 内完成：RequireWithdraw → 交易密码/TOTP → 白名单 →
// 再尝试 mesh 转发。mesh 失败时仍走本进程 withdraw.Request（冻结先于广播）。

// Author: Knox
// Date: 2021.08.01

package kcfund

import (
	"bytes"
	"io"
	    "strings"
	  "account-assets.kucoin.net/funding/internal/fundmesh"
	    "account-assets.kucoin.net/funding/internal/ledger"
	  "github.com/gin-gonic/gin"
)

   func (a *API) mesh() *fundmesh.Client {
	  if a == nil || a.rt == nil || a.rt.FundMesh == nil {
		  return nil
	   }
	   if !a.rt.FundMesh.Active() {
	    	  return nil
	  }
	   return a.rt.FundMesh
  }

func (a *API) markMeshDown(c *gin.Context, err error, path string) {
	   if a != nil && a.rt != nil {
	  	a.rt.Log.Warn().Err(err).Str("path", path).Msg("mesh downstream")
	   }
	    fail(c, 503, "downstream unavailable")
}

   func (a *API) meshOpts(c *gin.Context) fundmesh.CallOpts {
	  return fundmesh.CallOpts{
	  	   UserID:        a.user(c),
	   	   Authorization: strings.TrimSpace(c.GetHeader("Authorization")),
	     }
  }

// tryMeshAccounts 从 ledger-service 拉取 BookView 并映射为 Classic 账户行。
// 返回 false 表示调用方应使用 rt.Ledger.Accounts，避免在网关层合并两套余额来源。

   func (a *API) tryMeshAccounts(c *gin.Context, uid string) ([]kcAccount, bool, error) {
	   g := a.mesh()
	  if g == nil {
	    	  return nil, false, nil
	    }
	  books, err := g.LedgerAccounts(c.Request.Context(), a.meshOpts(c), uid)
	      if err != nil {
	  	  if a.rt != nil {
	   		   a.rt.Log.Warn().Err(err).Str("user", uid).Msg("mesh ledger accounts")
	   	  }
		   return nil, false, errDownstream
	   }
	      out := make([]kcAccount, 0, len(books))
	   for _, r := range books {
	  	  kc := bookToKC(r.Subject, r.Book)
		   out = append(out, kcAccount{
	   		   ID: uid + ":" + kc + ":" + r.Asset, Currency: r.Asset, Type: kc,
			  Balance: r.Total, Available: r.Available, Holds: r.Frozen,
		   })
	      }
	   return mergeKCAccounts(out), true, nil
   }

// tryMeshBills 从 ledger-service 拉流水；失败回落本进程 ListBills，禁止两套账单合并。
   func (a *API) tryMeshBills(c *gin.Context, uid string, limit int) ([]ledger.Bill, bool, error) {
	  g := a.mesh()
	  if g == nil {
		  return nil, false, nil
	  }
	      bills, err := g.LedgerBills(c.Request.Context(), a.meshOpts(c), uid, limit)
	   if err != nil {
		  if a.rt != nil {
			  a.rt.Log.Warn().Err(err).Str("user", uid).Msg("mesh ledger bills")
		  }
		   return nil, false, errDownstream
	       }
	      return bills, true, nil
  }

// forwardMeshDepositWithdraw 将 HTTP 原样转发至 deposit-withdraw。
// 成功时直接写响应并返回 true；传输层失败返回 false，由 rails 执行进程内 withdraw 路径。

  func (a *API) forwardMeshDepositWithdraw(c *gin.Context, method, path string, body []byte) bool {
	   g := a.mesh()
	  if g == nil {
		  return false
	     }
	     if body == nil && c != nil && c.Request != nil && c.Request.Body != nil {
		   raw, err := io.ReadAll(c.Request.Body)
	  	   if err != nil {
			  return false
	   	  }
		   body = raw
		   c.Request.Body = io.NopCloser(bytes.NewReader(body))
	  }
	     full := path
	  if c != nil && c.Request != nil {
	   	   q := strings.TrimSpace(c.Request.URL.RawQuery)
		  if q != "" {
	   		  full += "?" + q
	   	  }
	   }
	   code, raw, err := g.DepositWithdraw(c.Request.Context(), method, full, body, a.meshOpts(c))
	   if err != nil {
		   a.markMeshDown(c, err, path)
		   return true
	   }
	      if c != nil {
		  c.Data(code, "application/json", raw)
	      }
	   return true
  }

// meshDepositAddress 读路径充值地址；写路径仍由 addDepositAddress 调用 DepositAddress。
   func (a *API) meshDepositAddress(c *gin.Context, uid, chain string) (string, bool, error) {
	  g := a.mesh()
	  if g == nil {
		  return "", false, nil
	    }
	       view, err := g.DepositAddress(c.Request.Context(), a.meshOpts(c), uid, chain)
	     if err != nil {
	    	  if a.rt != nil {
			  a.rt.Log.Warn().Err(err).Str("user", uid).Str("chain", chain).Msg("mesh deposit address")
	  	   }
	  	  return "", false, errDownstream
	      }
	     if strings.TrimSpace(view.Address) == "" {
		  return "", false, nil
	    }
	      return view.Address, true, nil
   }

// tryMeshStatementExport 提交账单导出；失败时由 ledger_export 走 exjob 本进程队列。

   func (a *API) tryMeshStatementExport(c *gin.Context, body []byte) (int, []byte, bool) {
	       g := a.mesh()
	  if g == nil {
	    	  return 0, nil, false
	  }
	      code, raw, err := g.StatementExport(c.Request.Context(), body, a.meshOpts(c))
	  if err != nil {
		   a.markMeshDown(c, err, "statement-export")
	   	  return 0, nil, true
	      }
	   return code, raw, true
   }
