package okf

import (
	"testing"
	"time"
)

func TestTypedMetadata(t *testing.T) {
	timestamp := time.Date(2026, 10, 19, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	meta := Extract(map[string]interface{}{"timestamp": timestamp, "tags": []string{"one", "two"}})
	if meta.Timestamp != "2026-10-19T11:00:00Z" || len(meta.Tags) != 2 {
		t.Fatal(meta)
	}
}
