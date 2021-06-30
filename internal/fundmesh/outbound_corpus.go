package fundmesh

  func InvalidOutboundKeys() []string {
	   return []string{
		  "a",
		   "short",
		  "has space xxx",
		   "tab\tkeyxxxxxxx",
		   "fmout.v1.x.notakindbody",
		  "fmout.v2.c.legacyversionxx",
		   "fmout.v1.c.tiny",
		  "fmout.v1.",
		  "fmout.v1.c.",
		  "fmout.v1.d.",
		  "fmout.v1.c.!!!!bad",
		   "fmout.v1.e.abcdefgh",
		   "FMOUT.v1.c.clientkeyxx",
	  }
   }

  func InvalidOutboundAmounts() []string {
	   return []string{"", "-", "-1", "1.2.3", "1e2", "abc", "10,0", "NaN"}
  }

   func ValidBareClientKeys() []string {
	  return []string{
		  "client-key-alpha01",
		  "client-key-beta0002",
		  "wd-submit-gamma003",
		  "hdr-key-bbbbbb",
		   "amt-norm-cccccc",
	   }
   }
