package kcfund

import (
	    "strings"
	   "account-assets.kucoin.net/funding/internal/account"
	    "account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/subacct"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

  func (a *API) transfer(g *gin.RouterGroup) {
	   g.GET("/v1/accounts/transferable", a.getTransferQuotas)
	       g.GET("/v3/accounts/transfer-quotas", a.getTransferQuotas)
	   g.POST("/v3/accounts/universal-transfer", a.flexTransfer)
	  g.POST("/v2/accounts/inner-transfer", a.flexTransfer)
	   g.POST("/v1/accounts/inner-transfer", a.flexTransfer)
	      g.POST("/v2/accounts/sub-transfer", a.subTransfer)
	   g.GET("/v3/accounts/transfer-list", a.listTransfers)
	   g.GET("/v1/accounts/transfer-list", a.listTransfers)
   }

  func (a *API) listTransfers(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	    if !okU {
	  	   return
	   }
	   list, err := a.rt.Ledger.List(c.Request.Context(), uid, 0)
	     if err != nil {
		  fail(c, 400, err.Error())
		   return
	   }
	   from := strings.ToUpper(strings.TrimSpace(bffParam(c, "fromAccountType", "from")))
	   to := strings.ToUpper(strings.TrimSpace(bffParam(c, "toAccountType", "to")))
	      cur := strings.ToUpper(strings.TrimSpace(bffParam(c, "currency")))
	   items := make([]gin.H, 0)
	  for _, tr := range list {
	  	  if tr == nil || !inHistoryWindow(c, tr.CreatedAt) {
			   continue
		   }
	   	  switch tr.Direction.Settlement() {
		   case "book", "internal", "earn", "sub", "convert":
		  default:
	   		   continue
		  }
		  if cur != "" && strings.ToUpper(tr.Asset) != cur {
	   		  continue
	  	   }
	    	  item := transferListItemJSON(tr, railStatus(tr))
		  if from != "" {
	   		  if ft, _ := item["fromAccountType"].(string); ft != from && !aliasAccountTypeMatch(from, ft) {
	    			  continue
	  		  }
		  }
		  if to != "" {
	   		  if tt, _ := item["toAccountType"].(string); tt != to && !aliasAccountTypeMatch(to, tt) {
				  continue
			  }
		  }
		  items = append(items, item)
	      }
	    page, _ := parsePositiveInt(bffParam(c, "currentPage"), bffParam(c, "page"), 1)
	  pageSize, _ := parsePositiveInt(bffParam(c, "pageSize"), "", 50)
	  if pageSize > 200 {
		  pageSize = 200
	    }
	      total := len(items)
	  start := (page - 1) * pageSize
	  if start > total {
		  start = total
	  }
	  end := start + pageSize
	    if end > total {
		  end = total
	  }
	  totalPage := 1
	  if pageSize > 0 && total > 0 {
	   	  totalPage = (total + pageSize - 1) / pageSize
	  }
	  ok(c, gin.H{"items": items[start:end], "totalNum": total, "currentPage": page, "pageSize": pageSize, "totalPage": totalPage})
  }

  func (a *API) getTransferQuotas(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	   if !okU {
		  return
	  }
	  typ := bffParam(c, "type")
	   if strings.TrimSpace(typ) == "" {
		   typ = bffParam(c, "accountType")
	   }
	   if strings.TrimSpace(typ) == "" {
		   typ = "MAIN"
	   }
	     cur := strings.ToUpper(bffParam(c, "currency"))
	   symbol := strings.ToUpper(strings.TrimSpace(bffParam(c, "symbol")))
	      book, okb := kcToBook(typ)
	   if !okb {
		   fail(c, 400, "unknown account type")
	    	  return
	   }
	      // docs-new UTA transfer-quota：逐仓须带 symbol（如 BTC-USDT）。
	    if (strings.EqualFold(typ, "ISOLATED") || strings.EqualFold(typ, "ISOLATED_V2")) && symbol == "" {
	    	   fail(c, 400, "symbol required for isolated transfer quota")
		  return
	   }
	  if symbol != "" && (strings.EqualFold(typ, "ISOLATED") || strings.EqualFold(typ, "ISOLATED_V2")) {
		  pair, okP := a.isoBook().Get(uid, symbol)
	   	  if !okP {
			  fail(c, 404, "isolated account not found")
	   		  return
		  }
	  	  side := pair.Base
	    	  if cur == "" || strings.EqualFold(cur, pair.Quote.Currency) {
			  if cur == "" {
				  cur = pair.Quote.Currency
	   		  }
	  		  if strings.EqualFold(cur, pair.Quote.Currency) {
				  side = pair.Quote
			  }
	   	  }
		  if cur != "" && !strings.EqualFold(cur, side.Currency) && strings.EqualFold(cur, pair.Base.Currency) {
			  side = pair.Base
	    	  }
		  avail, _ := decimal.NewFromString(side.Available)
	    	  hold, _ := decimal.NewFromString(side.Hold)
		  bal := avail.Add(hold)
	   	  ok(c, gin.H{
			  "currency": side.Currency, "symbol": symbol, "balance": bal.String(),
	   		  "available": avail.String(), "holds": hold.String(), "hold": hold.String(),
			  "transferable": avail.String(),
	   	  })
	  	  return
	      }
	  rows, err := a.rt.Ledger.Accounts(c.Request.Context(), uid)
	   if err != nil {
		   fail(c, 400, err.Error())
		   return
	    }
	     want := ledgerBook(book)
	     kcWant := bookToKC(string(book), string(book))
	      avail,
		   hold := decimal.Zero, decimal.Zero
	       matched := false
	      for _, r := range rows {
	  	  if cur != "" && !strings.EqualFold(r.Asset, cur) {
			  continue
	    	  }
		  if r.Book != want && r.Subject != string(book) && bookToKC(r.Subject, r.Book) != kcWant {
	  		  continue
	   	  }
		  if cur == "" {
	   		   cur = r.Asset
	    	  } else if !strings.EqualFold(r.Asset, cur) {
	   		  continue
	  	   }
	    	  matched = true
	  	   if n, err := decimal.NewFromString(r.Available); err == nil {
			  avail = avail.Add(n)
		   }
	  	   if n, err := decimal.NewFromString(r.Frozen); err == nil {
	    		   hold = hold.Add(n)
	   	   }
	  }
	  if !matched && cur == "" && len(rows) > 0 {
	    	   avail, _ = decimal.NewFromString(rows[0].Available)
		  hold, _ = decimal.NewFromString(rows[0].Frozen)
		   cur = rows[0].Asset
	   }
	     bal := avail.Add(hold)
	      // UTA 轨返回 accountType + transferable；Classic 一致 docs-new balance/available/holds/transferable。
	   if strings.Contains(c.Request.URL.Path, "/ua/") {
		   out := gin.H{"accountType": strings.ToUpper(typ), "currency": cur, "transferable": avail.String()}
		   if symbol != "" {
			  out["symbol"] = symbol
	  	   }
		  ok(c, out)
	  	   return
	   }
	   ok(c, gin.H{
	    	   "currency": cur, "balance": bal.String(), "available": avail.String(),
		  "holds": hold.String(), "hold": hold.String(), "transferable": avail.String(),
	  })
  }

  func (a *API) flexTransfer(c *gin.Context) {
	  body, err := bindFlexTransferBody(c)
	  if err != nil {
		   fail(c, 400, err.Error())
	   	  return
	    }
	  if strings.TrimSpace(body.ClientOid) == "" {
	   	  fail(c, 400, "clientOid required")
	   	   return
	   }
	    amt, err := ledger.ParseAmount(body.Amount)
	   if err != nil {
		   fail(c, 400, err.Error())
	    	   return
	  }
	      if !amt.IsPositive() {
		   fail(c, 400, "amount must be positive")
	    	   return
	  }
	      uid, okU := a.requireUser(c)
	  if !okU {
	   	   return
	  }
	   kind := strings.ToUpper(strings.TrimSpace(body.Type))
	  if kind == "" {
	  	   kind = "INTERNAL"
	   }
	  switch kind {
	   case "INTERNAL":
		   if sameTransferBook(body.FromAccountType, body.ToAccountType) {
			   fail(c, 400, "fromAccountType must differ from toAccountType")
	  		   return
		   }
		   from, okf := kcToBook(body.FromAccountType)
		  to,
			   okt := kcToBook(body.ToAccountType)
		   if !okf || !okt {
			   fail(c, 400, "account type")
			   return
		  }
	    	  svc := account.New(a.rt.Ledger)
	   	   if _, err := svc.Open(c.Request.Context(), uid, from); err != nil {
			   fail(c, 400, err.Error())
	    		  return
	  	   }
		  if _, err := svc.Open(c.Request.Context(), uid, to); err != nil {
			  fail(c, 400, err.Error())
			   return
		  }
	    	  tr, err := svc.Move(c.Request.Context(), account.MoveReq{
			  UserID: uid, From: from, To: to, Asset: body.Currency,
	  		   Amount: amt, IdempotencyKey: body.ClientOid,
	    	  })
		  if err != nil {
			  fail(c, 400, err.Error())
	    		   return
	  	  }
		  if err := a.syncIsolated(uid, from, to, firstNonEmpty(body.Symbol, body.Tag), body.Currency, body.Amount); err != nil {
	   		   _, _ = svc.CompensateMove(c.Request.Context(), account.MoveReq{
	    			  UserID: uid, From: from, To: to, Asset: body.Currency,
				   Amount: amt, IdempotencyKey: body.ClientOid,
	  		   })
			   fail(c, 400, err.Error())
			  return
	   	  }
	  	   okAcceptedOrder(c, tr.ID, body.ClientOid)
		   a.emitAccountTransferEvent(c.Request.Context(), tr, kind,
			   strings.ToUpper(strings.TrimSpace(body.FromAccountType)),
			   strings.ToUpper(strings.TrimSpace(body.ToAccountType)), body.ClientOid)
	  case "PARENT_TO_SUB", "SUB_TO_PARENT":
		  a.hopTransfer(c, uid, kind, body.ClientOid, body.Currency, body.Amount, body.FromUserID, body.ToUserID)
	      default:
	    	   fail(c, 400, "transfer type")
	  }
   }

// hopTransfer 母子划转。出方向先扣母户账本再写 overlay；入方向先 hop/扣 overlay 再贷母户账本。禁止只写 overlay。

   func (a *API) hopTransfer(c *gin.Context, uid, kind, clientOid, currency, amount, fromUser, toUser string) {
	   svc := a.subs()
	  p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	      if err != nil {
		   fail(c, 400, err.Error())
		   return
	  }
	  from, to := p.ID, firstNonEmpty(toUser, fromUser)
	  if kind == "SUB_TO_PARENT" {
	   	   from, to = firstNonEmpty(fromUser, toUser), p.ID
	   }
	      from = resolveHopOwner(svc, p, from)
	       to = resolveHopOwner(svc, p, to)
	      if from == to {
		   fail(c, 400, "parent and sub account required")
	   	   return
	   }
	  if a.rt != nil && a.rt.FundSurf != nil && !a.rt.FundSurf.SubXferPerm(c.Request.Context(), p.ID, firstNonEmpty(from, to)) {
		   fail(c, 400, "sub-account transfer permission denied")
		   return
	     }
	      amt,
		   err := ledger.ParseAmount(amount)
	  if err != nil {
	    	   fail(c, 400, err.Error())
	  	   return
	   }
	   hop, err := HopFunds(c.Request.Context(), a.rt.Ledger, svc, HopFundsReq{
	    	  UserID: uid,
	  	   Kind:   kind, ClientOid: clientOid, Currency: currency,
		  Amount: amt, AmountRaw: amount, FromOwner: from, ToOwner: to,
		  QuotaParent: p.ID,
	      })
	       if err != nil {
		  fail(c, 400, err.Error())
		  return
	    }
	  okAcceptedOrder(c, hop.ID, clientOid)
	     a.emitAccountTransferHop(c.Request.Context(), hop, uid, kind, clientOid, currency, amount)
  }

  func (a *API) subTransfer(c *gin.Context) {
	  body, err := bindSubTransferBody(c)
	  if err != nil {
	    	   fail(c, 400, err.Error())
		  return
	   }
	      if strings.TrimSpace(body.ClientOid) == "" {
		   fail(c, 400, "clientOid required")
		   return
	     }
	     kind := strings.ToUpper(strings.TrimSpace(body.Type))
	  if kind == "" {
		  kind = "PARENT_TO_SUB"
	  }
	     switch kind {
	     case "PARENT_TO_SUB", "SUB_TO_PARENT":
	  default:
		  if strings.EqualFold(strings.TrimSpace(body.Direction), "IN") {
	    		  kind = "SUB_TO_PARENT"
	   	  } else {
			  kind = "PARENT_TO_SUB"
		  }
	    }
	      uid, okU := a.requireUser(c)
	       if !okU {
		   return
	       }
	  fromUser := firstNonEmpty(body.FromUserID, body.SubUserId)
	    toUser := firstNonEmpty(body.ToUserID, body.SubUserId)
	  a.hopTransfer(c, uid, kind, body.ClientOid, body.Currency, body.Amount, fromUser, toUser)
  }

 func aliasAccountTypeMatch(want, raw string) bool {
	  return strings.EqualFold(accountTypeJSON(want), accountTypeJSON(raw)) ||
		  strings.EqualFold(want, raw)
  }

 func (a *API) syncIsolated(uid string, from, to account.Book, symbol, currency, amount string) error {
	   if a == nil || a.rt == nil || a.rt.Isolated == nil {
	  	   return nil
	       }
	    switch {
	   case to == account.BookIsolated:
	    	   _, err := a.rt.Isolated.Credit(uid, symbol, currency, amount)
		  return err
	      case from == account.BookIsolated:
	  	   _, err := a.rt.Isolated.Debit(uid, symbol, currency, amount)
		  return err
	  }
	  return nil
  }

  func resolveHopOwner(svc *subacct.Service, p subacct.Parent, raw string) string {
	   raw = strings.TrimSpace(raw)
	  if raw == "" || raw == p.ID || raw == p.UserID {
		   return p.ID
	   }
	       if svc == nil {
		   return raw
	  }
	      for _, k := range svc.ListChildren(p.ID) {
		  if k.ID == raw || k.UserID == raw {
	  		   return k.ID
		   }
	   }
	       return raw
  }
