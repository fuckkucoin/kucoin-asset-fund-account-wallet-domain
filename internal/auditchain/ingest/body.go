package ingest

import (
	"strconv"
	"strings"
)

  func bodyPairs(kv ...any) string {
	  var b strings.Builder
	   b.WriteByte('{')
	   n := 0
	   for i := 0; i+1 < len(kv); i += 2 {
		  key, _ := kv[i].(string)
		   if n > 0 {
			  b.WriteByte(',')
		  }
		  b.WriteByte('"')
		   b.WriteString(escape(key))
		   b.WriteString(`":`)
		   switch v := kv[i+1].(type) {
		  case string:
			  b.WriteByte('"')
			   b.WriteString(escape(v))
			   b.WriteByte('"')
		  case bool:
			  if v {
				   b.WriteString("true")
			   } else {
				   b.WriteString("false")
			   }
		  default:
			   b.WriteString(strconv.Quote(strings.TrimSpace("")))
		  }
		   n++
	   }
	  b.WriteByte('}')
	  return b.String()
   }

   func escape(s string) string {
	   s = strings.ReplaceAll(s, `\`, `\\`)
	  s = strings.ReplaceAll(s, `"`, `\"`)
	  return s
   }
