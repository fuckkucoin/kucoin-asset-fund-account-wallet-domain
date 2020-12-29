// @FileName: convert_bff.go
// @Summary: 闪兑 BFF 入参绑定。
// @Author: Cason
// @Created: 2020.12.28

package kcfund

import (
	"io"
	            "strings"

	    "github.com/gin-gonic/gin"
)

type convertQuoteBody struct {
	FromCurrency string `json:"fromCurrency"`
	    ToCurrency   string `json:"toCurrency"`
	    FromAmount   string `json:"fromAmount"`
	ToAmount     string `json:"toAmount"`
	AccountType  string `json:"accountType"`
	      ClientOid    string `json:"clientOid"`
}

   func bindConvertQuoteBody(c *gin.Context) (convertQuoteBody, error) {
	    var body convertQuoteBody
	      if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	        return body, err
	              }
	        body.FromCurrency = strings.TrimSpace(firstNonEmpty(body.FromCurrency, bffParam(c, "from", "fromCurrency")))
	    body.ToCurrency = strings.TrimSpace(firstNonEmpty(body.ToCurrency, bffParam(c, "to", "toCurrency")))
	    body.FromAmount = strings.TrimSpace(firstNonEmpty(body.FromAmount, bffParam(c, "fromSize", "fromAmount", "from_amount")))
	        body.ToAmount = strings.TrimSpace(firstNonEmpty(body.ToAmount, bffParam(c, "toSize", "toAmount", "to_amount")))
	        body.AccountType = strings.TrimSpace(firstNonEmpty(body.AccountType, bffParam(c, "accountType", "account_type")))
	    body.ClientOid = strings.TrimSpace(firstNonEmpty(body.ClientOid, bffParam(c, "clientOid", "client_oid")))
	return body, nil
}

type convertPlaceBody struct {
	FromCurrency string `json:"fromCurrency"`
	        ToCurrency   string `json:"toCurrency"`
	    FromAmount   string `json:"fromAmount"`
	      Rate         string `json:"rate"`
	            AccountType  string `json:"accountType"`
	      QuoteID      string `json:"quoteId"`
	    ClientOid    string `json:"clientOid"`
	ConvertType  string `json:"convertType"`
	      SlippageBps  string `json:"slippageBps"`
}

   func bindConvertPlaceBody(c *gin.Context) (convertPlaceBody, error) {
	    var body convertPlaceBody
	         if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	    	return body, err
	   }
	    body.FromCurrency = strings.TrimSpace(firstNonEmpty(body.FromCurrency, bffParam(c, "from", "fromCurrency")))
	body.ToCurrency = strings.TrimSpace(firstNonEmpty(body.ToCurrency, bffParam(c, "to", "toCurrency")))
	          body.FromAmount = strings.TrimSpace(firstNonEmpty(body.FromAmount, bffParam(c, "fromSize", "fromAmount")))
	body.Rate = strings.TrimSpace(firstNonEmpty(body.Rate, bffParam(c, "rate")))
	  body.AccountType = strings.TrimSpace(firstNonEmpty(body.AccountType, bffParam(c, "accountType")))
	body.QuoteID = strings.TrimSpace(firstNonEmpty(body.QuoteID, bffParam(c, "quoteId", "quote_id")))
	body.ClientOid = strings.TrimSpace(firstNonEmpty(body.ClientOid, bffParam(c, "clientOid", "client_oid")))
	   body.ConvertType = strings.TrimSpace(firstNonEmpty(body.ConvertType, bffParam(c, "convertType")))
	   body.SlippageBps = strings.TrimSpace(bffParam(c, "slippageBps", "slippage_bps"))
	      return body, nil
}

func convertListQuery(c *gin.Context) (page, pageSize int) {
	      page, _ = parsePositiveInt(bffParam(c, "currentPage"), bffParam(c, "page"), 1)
	pageSize, _ = parsePositiveInt(bffParam(c, "pageSize"), "", 20)
	        if pageSize > 100 {
	       	pageSize = 100
	   }
	return
}

func limitQuoteParams(c *gin.Context) (from, to, fromSize, toSize string) {
	   from = strings.TrimSpace(firstNonEmpty(bffParam(c, "fromCurrency"), bffParam(c, "from")))
	         to = strings.TrimSpace(firstNonEmpty(bffParam(c, "toCurrency"), bffParam(c, "to")))
	fromSize = strings.TrimSpace(firstNonEmpty(bffParam(c, "fromCurrencySize"), bffParam(c, "fromSize", "amount")))
	   toSize = strings.TrimSpace(bffParam(c, "toCurrencySize", "toSize"))
	       return
}
