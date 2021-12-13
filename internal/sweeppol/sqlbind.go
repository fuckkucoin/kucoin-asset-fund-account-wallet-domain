package sweeppol

import (
	"context"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"github.com/google/uuid"
)

   func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	  if b != nil {
		   b.sql = db
	  }
	  return b
   }

  func (b *Book) sqlOn() bool {
	   return b != nil && b.sql != nil && b.sql.DB != nil
   }

   func formatWeekdays(days []time.Weekday) string {
	   if len(days) == 0 {
		   return ""
	  }
	   parts := make([]string, 0, len(days))
	  for _, d := range days {
		  parts = append(parts,
			  strconv.Itoa(int(d)))
	   }
	  return strings.Join(parts, ",")
  }

  func (b *Book) PersistSchedule(sch Schedule) error {
	   if !b.sqlOn() {
		   return nil
	  }
	  if b.schedID == "" {
		   b.schedID = uuid.NewString()
	   }
	  return sqlrepo.NewSweep(b.sql).PutSchedule(context.Background(), sqlrepo.SweepSchedule{
		  ID: b.schedID, Region: sch.Region, Weekdays: formatWeekdays(sch.Window.Weekdays),
		   StartUTC: sch.Window.StartUTC,
		  EndUTC: sch.Window.EndUTC, MaxPlans: sch.MaxPlans, Halt: sch.Halt,
		   Updated: time.Now().UTC(),
	  })
   }

   func (b *Book) persistPlans(plans []Plan) error {
	   if !b.sqlOn() {
		   return nil
	   }
	   repo := sqlrepo.NewSweep(b.sql)
	  for _, p := range plans {
		  if err := repo.PutPlan(context.Background(), sqlrepo.SweepPlanRow{
			  ScheduleID: b.schedID, Asset: p.Asset, Amount: p.Amount, Reason: p.Reason, State: "planned",
		  }); err != nil {
			   return err
		  }
	   }
	  return nil
  }
