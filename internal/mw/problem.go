package mw

import (
	"net/http"
	"strconv"
	"account-assets.kucoin.net/funding/internal/apierr"
	"github.com/gin-gonic/gin"
)

// Problem RFC 7807 形的错误体。handler 只传 error，状态码走 apierr。
type Problem struct {
	Type string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

   func ProblemOf(c *gin.Context, err error) Problem {
	   st := apierr.HTTPStatus(err)
	  inst := ""
	  if c != nil {
		  inst = c.FullPath()
		   if inst == "" && c.Request != nil {
			  inst = c.Request.URL.Path
		   }
	   }
	   return Problem{
		   Type: "https://query-gateway.kucoin.net/problems/" + strconv.Itoa(st),
		   Title: http.StatusText(st),
		  Status: st,
		  Detail: err.Error(),
		  Instance: inst,
	  }
   }

  func WriteProblem(c *gin.Context, err error) {
	   if err == nil {
		  c.JSON(http.StatusOK, gin.H{"ok": true})
		  return
	  }
	  p := ProblemOf(c, err)
	  c.JSON(p.Status, p)
  }

  func WriteOK(c *gin.Context, body any) {
	   c.JSON(http.StatusOK, body)
   }
