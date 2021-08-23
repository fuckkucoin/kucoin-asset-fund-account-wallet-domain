// @FileName: earn_bff.go
// @Summary: 理财 BFF 入参：body + query/query= 别名。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	      "io"
	        "strings"

	    "github.com/gin-gonic/gin"
)

type earnPurchaseBody struct {
	      ProductID      string `json:"productId"`
	      Currency       string `json:"currency"`
	Asset          string `json:"asset"`
	Amount         string `json:"amount"`
	        ClientOid      string `json:"clientOid"`
	        IdempotencyKey string `json:"idempotencyKey"`
	AutoSubscribe  bool   `json:"autoSubscribe"`
}

   func bindEarnPurchaseBody(c *gin.Context) (earnPurchaseBody, error) {
	var body earnPurchaseBody
	      if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	        	  return body, err
	      }
	  body.ProductID = strings.TrimSpace(firstNonEmpty(body.ProductID, bffParam(c, "productId", "product_id")))
	    body.Currency = strings.TrimSpace(firstNonEmpty(body.Currency, firstNonEmpty(body.Asset, bffParam(c, "currency", "asset"))))
	    body.Amount = strings.TrimSpace(firstNonEmpty(body.Amount, bffParam(c, "amount")))
	    body.ClientOid = strings.TrimSpace(firstNonEmpty(body.ClientOid, firstNonEmpty(body.IdempotencyKey,
	   bffParam(c, "clientOid", "client_oid", "idempotencyKey", "idempotency_key"))))
	if v, ok := bffParamBool(c, "autoSubscribe", "auto_subscribe"); ok {
	                body.AutoSubscribe = v
	}
	              return body, nil
}

type earnRedeemBody struct {
	OrderID        string `json:"orderId"`
	      PositionID     string `json:"positionId"`
	ClientOid      string `json:"clientOid"`
	      IdempotencyKey string `json:"idempotencyKey"`
}

  func bindEarnRedeemBody(c *gin.Context) (earnRedeemBody, error) {
	var body earnRedeemBody
	        if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	         return body, err
	        }
	            body.OrderID = strings.TrimSpace(firstNonEmpty(body.OrderID, firstNonEmpty(body.PositionID,
		bffParam(c, "orderId", "order_id", "positionId", "position_id"))))
	body.ClientOid = strings.TrimSpace(firstNonEmpty(body.ClientOid, firstNonEmpty(body.IdempotencyKey,
	        	bffParam(c, "clientOid", "client_oid", "idempotencyKey", "idempotency_key"))))
	    return body, nil
}

type earnAutoBody struct {
	        OrderID       string `json:"orderId"`
	              AutoSubscribe bool   `json:"autoSubscribe"`
}

   func bindEarnAutoBody(c *gin.Context) (earnAutoBody, error) {
	var body earnAutoBody
	           if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	    	return body, err
	      }
	    body.OrderID = strings.TrimSpace(firstNonEmpty(body.OrderID, bffParam(c, "orderId", "order_id", "positionId")))
	        if v, ok := bffParamBool(c, "autoSubscribe", "auto_subscribe"); ok {
	    	body.AutoSubscribe = v
	}
	return body, nil
}

func earnProductCategory(c *gin.Context) string {
	return strings.ToLower(strings.TrimSpace(bffParam(c, "category")))
}
