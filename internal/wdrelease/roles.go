package wdrelease

  func RoleForLevel(level int) string {
	   switch level {
	  case 1:
		   return "reviewer"
	  case 2:
		  return "senior"
	   default:
		   if level <= 0 {
			   return ""
		   }
		   return "duty"
	   }
  }

   func KnownRole(role string) bool {
	  switch role {
	   case "reviewer", "senior", "duty":
		   return true
	  default:
		   return false
	  }
  }

  func roleMatches(level int, role string) bool {
	   want := RoleForLevel(level)
	   if want == "" {
		  return false
	   }
	   return role == want
   }
