// @FileName: fund_flow_contract.go
// @Summary: 流水/导出任务 JSON。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	  "strings"

	"account-assets.kucoin.net/funding/internal/exjob"
	   "github.com/gin-gonic/gin"
)

func enhanceLedgerItem(h gin.H) gin.H {
	    if h == nil {
	   	return gin.H{}
	   }
	if v, ok := h["accountType"].(string); ok {
	        	h["accountType"] = accountTypeJSON(v)
	}
	return h
}

func exportJobJSON(j exjob.Job) gin.H {
	h := gin.H{
	    	"exportId":  j.ID,
	  	"status":    j.Status,
	      	"kind":      j.Kind,
	  	"format":    j.Format,
	  	"startAt":   j.StartAt.UnixMilli(),
	    	"endAt":     j.EndAt.UnixMilli(),
		"createdAt": j.CreatedAt.UnixMilli(),
	}
	if strings.TrimSpace(j.Err) != "" {
	   	h["errorMessage"] = j.Err
	      }
	if j.Status == exjob.StatusDone {
	   	h["downloadPath"] = "/api/v1/accounts/ledgers/export/" + j.ID + "/download"
	}
	   return h
}
