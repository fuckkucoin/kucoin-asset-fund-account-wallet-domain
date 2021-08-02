// @FileName: fund_flow_bff.go
// @Summary: 流水导出 / 资金单 query 别名。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	      "io"
	      "strings"

	"github.com/gin-gonic/gin"
)

type ledgerExportBody struct {
	      Kind    string `json:"kind"`
	    Format  string `json:"format"`
	      StartAt string `json:"startAt"`
	EndAt   string `json:"endAt"`
}

  func bindLedgerExportBody(c *gin.Context) (ledgerExportBody, error) {
	var body ledgerExportBody
	        if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	      return body, err
	                }
	      body.Kind = strings.TrimSpace(firstNonEmpty(body.Kind, bffParam(c, "kind")))
	    body.Format = strings.TrimSpace(firstNonEmpty(body.Format, bffParam(c, "format")))
	            body.StartAt = strings.TrimSpace(firstNonEmpty(body.StartAt, bffParam(c, "startAt", "start_at")))
	body.EndAt = strings.TrimSpace(firstNonEmpty(body.EndAt, bffParam(c, "endAt", "end_at")))
	return body, nil
}

  func fundOrderQuery(c *gin.Context) (kind, cur, status string, page, pageSize int, startAt, endAt string) {
	kind = strings.ToLower(strings.TrimSpace(bffParam(c, "kind")))
	    if kind == "" {
	   kind = "all"
	    }
	cur = strings.ToUpper(strings.TrimSpace(bffParam(c, "currency")))
	    status = strings.ToUpper(strings.TrimSpace(bffParam(c, "status")))
	    page, _ = parsePositiveInt(bffParam(c, "currentPage"), bffParam(c, "page"), 1)
	pageSize, _ = parsePositiveInt(bffParam(c, "pageSize"), "", 20)
	startAt = bffParam(c, "startAt")
	endAt = bffParam(c, "endAt")
	return
}
