package clearing

import (
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterCalendarOps 注册日历配置路由。

   func RegisterCalendarOps(v1 *gin.RouterGroup, cal *Calendar) {
	  if cal == nil {
		  cal = NewCalendar()
	  }
	  // 列出所有假日
	  v1.GET("/clearing/calendar/holidays", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{
			  "holidays": cal.List(),
			   "count": len(cal.List()),
		  })
	  })
	  // 添加假日
	  v1.POST("/clearing/calendar/holidays", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  Day string `json:"day"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if err := cal.Add(body.Day); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"holidays": cal.List()})
	  })
	  // 移除假日
	  v1.DELETE("/clearing/calendar/holidays/:day", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   day := strings.TrimSpace(c.Param("day"))
		  cal.Remove(day)
		  c.JSON(http.StatusOK, gin.H{"holidays": cal.List()})
	   })
	   // 查询指定日期是否为工作日
	  v1.GET("/clearing/calendar/bizday", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   day := c.Query("day")
		  if day == "" {
			  day = time.Now().UTC().Format("2006-01-02")
		   }
		  t, err := time.Parse("2006-01-02", day)
		   if err != nil {
			   mw.WriteProblem(c, ErrHoliday)
			  return
		   }
		  isHoliday := cal.Has(t)
		  isWeekend := t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
		  isBizDay := !isHoliday && !isWeekend
		  c.JSON(http.StatusOK, gin.H{
			  "day":        day,
			  "is_biz_day": isBizDay,
			   "is_holiday": isHoliday,
			   "is_weekend": isWeekend,
			   "weekday":    t.Weekday().String(),
		  })
	  })
	  // 查询下一工作日
	  v1.GET("/clearing/calendar/next-bizday", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  fromDay := c.Query("from")
		   if fromDay == "" {
			  fromDay = time.Now().UTC().Format("2006-01-02")
		   }
		  t, err := time.Parse("2006-01-02", fromDay)
		   if err != nil {
			   mw.WriteProblem(c, ErrHoliday)
			   return
		  }
		  next := nextBizDayFromCal(t, cal)
		   c.JSON(http.StatusOK, gin.H{
			  "from":         fromDay,
			  "next_biz_day": next.Format("2006-01-02"),
			   "weekday": next.Weekday().String(),
		   })
	   })
   }

// nextBizDayFromCal 按日历计算下一工作日。

   func nextBizDayFromCal(from time.Time, cal *Calendar) time.Time {
	   next := from.Add(24 * time.Hour)
	  for {
		  isWeekend := next.Weekday() == time.Saturday || next.Weekday() == time.Sunday
		  isHoliday := cal != nil && cal.Has(next)
		   if !isWeekend && !isHoliday {
			   return next
		  }
		  next = next.Add(24 * time.Hour)
	  }
   }
