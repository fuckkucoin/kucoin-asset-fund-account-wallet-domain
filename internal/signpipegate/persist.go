package signpipegate

import "strings"

   func cloneLease(l *InflightLease) *InflightLease {
	   if l == nil {
		   return nil
	   }
	  cp := l.Copy()
	   return &cp
   }

  func cloneFault(f *FaultCase) *FaultCase {
	   if f == nil {
		  return nil
	  }
	  cp := f.Copy()
	   return &cp
   }

   func cloneCloseout(c *CloseoutProof) *CloseoutProof {
	  if c == nil {
		   return nil
	  }
	  cp := c.Copy()
	   return &cp
   }

  func cloneArtifact(a *SignedArtifact) *SignedArtifact {
	   if a == nil {
		   return nil
	   }
	  cp := a.Copy()
	  return &cp
  }

  func emptyID(id string) bool {
	   return strings.TrimSpace(id) == ""
  }

  func requireID(id string, missing error) error {
	   if emptyID(id) {
		   return missing
	   }
	   return nil
  }
