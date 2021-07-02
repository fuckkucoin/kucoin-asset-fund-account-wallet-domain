package forbidgate

import "encoding/json"

  func marshalJSON(v any) ([]byte, error) {
	   return json.Marshal(v)
   }

   func unmarshalJSON(data []byte, v any) error {
	   if len(data) == 0 {
		   return nil
	  }
	  return json.Unmarshal(data, v)
  }
