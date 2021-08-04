// @FileName: bff_query.go
// @Summary: 解析 query：显式键先，其次 ?query= 中的 URL 编码或 JSON 键值。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	  "bytes"
	    "encoding/json"
	"io"
	    "net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

  func bffOpaqueQuery(c *gin.Context) url.Values {
	      if c == nil {
	    return nil
	          }
	         raw := strings.TrimSpace(c.Query("query"))
	if raw == "" {
	       return nil
	}
	if strings.HasPrefix(raw, "{") {
	  	var flat map[string]json.RawMessage
	        	if json.Unmarshal([]byte(raw), &flat) == nil && len(flat) > 0 {
	       	  v := make(url.Values)
	    		for k, rm := range flat {
				var s string
	  			if json.Unmarshal(rm, &s) == nil {
	    			  v.Set(k, s)
					continue
				}
	  			var b bool
	  			if json.Unmarshal(rm, &b) == nil {
					v.Set(k, strconv.FormatBool(b))
					continue
	    			}
				v.Set(k, strings.Trim(string(rm), `"`))
	        		}
	      		return v
	  	}
	        }
	    s := raw
	if strings.HasPrefix(s, "?") {
	  	s = s[1:]
	    }
	    v, err := url.ParseQuery(s)
	if err != nil || len(v) == 0 {
		return nil
	}
	        return v
}

// bffParam 先读显式 query，再读 BFF 打包的 query=。
func bffParam(c *gin.Context, keys ...string) string {
	      for _, k := range keys {
	          	if v := strings.TrimSpace(c.Query(k)); v != "" {
	    		return v
	   	}
	}
	extra := bffOpaqueQuery(c)
	        if extra == nil {
	        	return ""
	          }
	  for _, k := range keys {
	          	if v := strings.TrimSpace(extra.Get(k)); v != "" {
	   		return v
		}
	}
	return ""
}

func bffParamBool(c *gin.Context, keys ...string) (bool, bool) {
	   s := bffParam(c, keys...)
	        if s == "" {
	         	return false, false
	    }
	    b, err := strconv.ParseBool(s)
	    if err != nil {
	      	return false, false
	          }
	    return b, true
}

func bindJSONOrEmpty(c *gin.Context, dst any) error {
	if c == nil || c.Request == nil || c.Request.Body == nil {
	        	return io.EOF
	        }
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
	    	return err
	        }
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	         trim := bytes.TrimSpace(raw)
	if len(trim) == 0 {
	        	return io.EOF
	      }
	return json.Unmarshal(trim, dst)
}
