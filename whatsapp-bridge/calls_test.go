package main

import (
	"testing"
	"time"
)

func TestTerminateStatus(t *testing.T) {
	cases := []struct {
		answered bool
		reason   string
		want     string
	}{
		{true, "", "answered"},
		{true, "timeout", "answered"},
		{false, "timeout", "missed"},
		{false, "reject", "rejected"},
		{false, "rejected", "rejected"},
		{false, "", "missed"},
	}
	for _, c := range cases {
		if got := terminateStatus(c.answered, c.reason); got != c.want {
			t.Errorf("terminateStatus(%v,%q)=%q want %q", c.answered, c.reason, got, c.want)
		}
	}
}

func TestCallDurationSeconds(t *testing.T) {
	acc := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	end := acc.Add(95 * time.Second)
	if got := callDurationSeconds(&acc, &end); got != 95 {
		t.Errorf("duration = %d want 95", got)
	}
	if got := callDurationSeconds(nil, &end); got != 0 {
		t.Errorf("nil accept => %d want 0", got)
	}
	earlier := acc.Add(-10 * time.Second)
	if got := callDurationSeconds(&acc, &earlier); got != 0 {
		t.Errorf("negative => %d want 0", got)
	}
}

func TestCallLifecycle_Answered(t *testing.T) {
	store, err := newMessageStoreAt(t.TempDir() + "/messages.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	if err := store.RecordCallOffer("CALL1", "111@s.whatsapp.net", "111", "voice", "incoming", start); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCallAccept("CALL1", start.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCallTerminate("CALL1", start.Add(125*time.Second), ""); err != nil {
		t.Fatal(err)
	}

	calls, err := store.GetCalls("111@s.whatsapp.net", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("got %d calls want 1", len(calls))
	}
	c := calls[0]
	if c.Status != "answered" {
		t.Errorf("status=%q want answered", c.Status)
	}
	if c.DurationSeconds != 120 {
		t.Errorf("duration=%d want 120", c.DurationSeconds)
	}
	if c.CallType != "voice" || c.Direction != "incoming" {
		t.Errorf("type/direction = %q/%q", c.CallType, c.Direction)
	}
}

func TestCallLifecycle_MissedNoAccept(t *testing.T) {
	store, err := newMessageStoreAt(t.TempDir() + "/messages.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	if err := store.RecordCallOffer("CALL2", "222@s.whatsapp.net", "222", "voice", "incoming", start); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCallTerminate("CALL2", start.Add(30*time.Second), "timeout"); err != nil {
		t.Fatal(err)
	}
	calls, _ := store.GetCalls("222@s.whatsapp.net", 10)
	if len(calls) != 1 || calls[0].Status != "missed" || calls[0].DurationSeconds != 0 {
		t.Fatalf("got %+v want status=missed duration=0", calls)
	}
}
