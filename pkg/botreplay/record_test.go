//go:build goldens_record

package botreplay

import (
	"context"
	"testing"
)

// TestRecordGoldens re-records every wired scenario by hitting the real
// LLM and overwrites the committed fixtures under testdata/. It is gated
// behind the `goldens_record` build tag and requires provider
// credentials, so it never runs in CI. Invoke via:
//
//	task test:goldens:record
//
// Scenarios whose backend credentials are missing fail their subtest
// (loudly) rather than silently producing an empty fixture — record is a
// deliberate, supervised operation.
func TestRecordGoldens(t *testing.T) {
	for _, s := range Scenarios() {
		t.Run(s.Bot+"/"+s.Name, func(t *testing.T) {
			f, err := Record(context.Background(), s)
			if err != nil {
				t.Fatalf("record %s/%s: %v", s.Bot, s.Name, err)
			}
			path := s.FixturePath()
			if err := f.Save(path); err != nil {
				t.Fatalf("save %s: %v", path, err)
			}
			t.Logf("recorded %s (%s/%s)", path, f.Backend, f.Model)
		})
	}
}
