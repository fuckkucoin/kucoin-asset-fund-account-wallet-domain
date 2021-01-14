// @FileName: transfer_bff.go
// @Summary: 划转 BFF body / query 别名。
// @Author: Cason
// @Created: 2021.01.13

package kcfund

import (
	        "io"
	        "strings"

	      "github.com/gin-gonic/gin"
)

type flexTransferBody struct {
	    ClientOid       string `json:"clientOid"`
	          Currency        string `json:"currency"`
	Amount          string `json:"amount"`
	FromAccountType string `json:"fromAccountType"`
	      ToAccountType   string `json:"toAccountType"`
	Type            string `json:"type"`
	    FromUserID      string `json:"fromUserId"`
	ToUserID        string `json:"toUserId"`
	        Tag             string `json:"tag"`
	Symbol          string `json:"symbol"`
}

  func bindFlexTransferBody(c *gin.Context) (flexTransferBody, error) {
	var body flexTransferBody
	if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	   return body, err
	      }
	        body.ClientOid = strings.TrimSpace(firstNonEmpty(body.ClientOid, bffParam(c, "clientOid", "client_oid")))
	      body.Currency = strings.TrimSpace(firstNonEmpty(body.Currency, bffParam(c, "currency")))
	body.Amount = strings.TrimSpace(firstNonEmpty(body.Amount, bffParam(c, "amount")))
	    body.FromAccountType = strings.TrimSpace(firstNonEmpty(body.FromAccountType, bffParam(c, "fromAccountType", "from")))
	body.ToAccountType = strings.TrimSpace(firstNonEmpty(body.ToAccountType, bffParam(c, "toAccountType", "to")))
	    body.Type = strings.TrimSpace(firstNonEmpty(body.Type, bffParam(c, "type", "transferType")))
	        body.FromUserID = strings.TrimSpace(firstNonEmpty(body.FromUserID, bffParam(c, "fromUserId", "from_user_id")))
	body.ToUserID = strings.TrimSpace(firstNonEmpty(body.ToUserID, bffParam(c, "toUserId", "to_user_id")))
	        body.Tag = strings.TrimSpace(firstNonEmpty(body.Tag, bffParam(c, "tag")))
	    body.Symbol = strings.TrimSpace(firstNonEmpty(body.Symbol, bffParam(c, "symbol")))
	if body.ClientOid == "" {
	            	body.ClientOid = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	    return body, nil
}

type subTransferBody struct {
	        ClientOid      string `json:"clientOid"`
	      Currency       string `json:"currency"`
	Amount         string `json:"amount"`
	        Direction      string `json:"direction"`
	AccountType    string `json:"accountType"`
	    SubAccountType string `json:"subAccountType"`
	SubUserId      string `json:"subUserId"`
	Type           string `json:"type"`
	      FromUserID     string `json:"fromUserId"`
	ToUserID       string `json:"toUserId"`
}

func bindSubTransferBody(c *gin.Context) (subTransferBody, error) {
	var body subTransferBody
	        if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	        	return body, err
	    }
	body.ClientOid = strings.TrimSpace(firstNonEmpty(body.ClientOid, bffParam(c, "clientOid", "client_oid")))
	          body.Currency = strings.TrimSpace(firstNonEmpty(body.Currency, bffParam(c, "currency")))
	      body.Amount = strings.TrimSpace(firstNonEmpty(body.Amount, bffParam(c, "amount")))
	          body.Direction = strings.TrimSpace(firstNonEmpty(body.Direction, bffParam(c, "direction")))
	body.SubUserId = strings.TrimSpace(firstNonEmpty(body.SubUserId, bffParam(c, "subUserId", "sub_user_id")))
	body.Type = strings.TrimSpace(firstNonEmpty(body.Type, bffParam(c, "type")))
	    body.FromUserID = strings.TrimSpace(firstNonEmpty(body.FromUserID, bffParam(c, "fromUserId")))
	body.ToUserID = strings.TrimSpace(firstNonEmpty(body.ToUserID, bffParam(c, "toUserId")))
	      if body.ClientOid == "" {
		body.ClientOid = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	        }
	        return body, nil
}

func sameTransferBook(a, b string) bool {
	          from, okf := kcToBook(a)
	            to, okt := kcToBook(b)
	      return okf && okt && from == to
}
