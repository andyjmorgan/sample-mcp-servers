package mcpserve

import (
	"encoding/json"
	"testing"
)

func TestToolSetsTitleAndAllHints(t *testing.T) {
	for _, idem := range []bool{true, false} {
		b, err := json.Marshal(Tool("x", "X Tool", "desc", idem))
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Title       string         `json:"title"`
			Annotations map[string]any `json:"annotations"`
		}
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got.Title != "X Tool" || got.Annotations["title"] != "X Tool" {
			t.Errorf("title not set: %s", b)
		}
		want := map[string]bool{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": idem, "openWorldHint": false}
		for k, v := range want {
			if gv, ok := got.Annotations[k]; !ok || gv != v {
				t.Errorf("%s = %v (present %v), want %v: %s", k, gv, ok, v, b)
			}
		}
	}
}
