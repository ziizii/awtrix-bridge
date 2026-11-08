package bridge

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestRejections(t *testing.T) {
	var buf bytes.Buffer
	r := newRejections(slog.New(slog.NewTextHandler(&buf, nil)))
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	bad := []byte(`{"ok":false,"error":{"code":"validationFailed","message":"unknown key \"progressBC\"","field":"progressBC"}}`)

	for i := range 3 {
		r.note("awtrix/cmd/apps/pushed/timer/result", bad, now.Add(time.Duration(i)*time.Second))
	}
	r.note("awtrix/cmd/apps/pushed/solar/result", []byte(`{"ok":true}`), now)
	r.note("awtrix/cmd/apps/pushed/solar/result", []byte(`not json`), now)

	got := r.list()
	if len(got) != 1 || got[0].Topic != "awtrix/cmd/apps/pushed/timer" || got[0].Count != 3 ||
		got[0].Reason != `unknown key "progressBC"` || !got[0].At.Equal(now.Add(2*time.Second)) {
		t.Fatalf("list: %+v", got)
	}
	if n := strings.Count(buf.String(), "clock rejected command"); n != 1 {
		t.Errorf("logged %d warnings, want 1:\n%s", n, buf.String())
	}

	r.note("awtrix/cmd/apps/pushed/timer/result", []byte(`{"ok":true}`), now)
	if len(r.list()) != 0 || !strings.Contains(buf.String(), "clock accepts command again") {
		t.Errorf("not cleared: %+v\n%s", r.list(), buf.String())
	}
}
