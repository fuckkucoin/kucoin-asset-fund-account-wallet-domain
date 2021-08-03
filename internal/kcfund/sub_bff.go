// @FileName: sub_bff.go
// @Summary: 主子账户 BFF 入参。
// @Author: Cason
// @Created: 2021.08.02

package kcfund

import (
	"io"
	"strings"

	"github.com/gin-gonic/gin"
)

type createSubBody struct {
	    SubName  string `json:"subName"`
	Password string `json:"password"`
	Remarks  string `json:"remarks"`
	        Access   string `json:"access"`
}

   func bindCreateSubBody(c *gin.Context) (createSubBody, error) {
	        var body createSubBody
	              body.Access = strings.TrimSpace(firstNonEmpty(body.Access, bffParam(c, "access")))
	body.Password = strings.TrimSpace(firstNonEmpty(body.Password, bffParam(c, "password")))
	            return body, nil
}

type subTransferPermBody struct {
	      SubUserID    string `json:"subUserId"`
	        Transferable bool   `json:"transferable"`
}

func bindSubTransferPermBody(c *gin.Context, pathSubID string) (subTransferPermBody, error) {
	var body subTransferPermBody
	if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	            	return body, err
	}
	        body.SubUserID = strings.TrimSpace(firstNonEmpty(body.SubUserID, firstNonEmpty(pathSubID, bffParam(c, "subUserId", "sub_user_id"))))
	if b, ok := bffParamBool(c, "transferable"); ok {
	      	body.Transferable = b
	          }
	      return body, nil
}

type subFreezeBody struct {
	Frozen *bool  `json:"frozen"`
	Reason string `json:"reason"`
}

func bindSubFreezeBody(c *gin.Context) (subFreezeBody, bool, error) {
	    var body subFreezeBody
	        if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
		return body, false, err
	    }
	            if body.Frozen == nil {
	    	if b, ok := bffParamBool(c, "frozen"); ok {
	      		body.Frozen = &b
	        	}
	    }
	              body.Reason = strings.TrimSpace(firstNonEmpty(body.Reason, bffParam(c, "reason")))
	      if body.Frozen == nil {
	      	return body, false, nil
	    }
	            return body, true, nil
}
