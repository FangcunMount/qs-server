package compatibilityretirementstop

import (
	"encoding/json"
	"testing"
)

func TestRemoteSessionRequestGateAllowsTheFixedOwnedPurgeSequence(t *testing.T) {
	// Exercise the parser and the exact request gate used by the native remote
	// loop. These local state transitions are only request eligibility; they do
	// not issue a native grant, Lease, runtime observation or delete capability.
	var bound, stopped, controlledIssued, resumed bool
	for i, action := range []string{"bind", "stop", "controlled_resume", "check_running", "purge_materials"} {
		t.Run(action, func(t *testing.T) {
			seq := uint64(i + 1)
			raw, e := json.Marshal(SessionRequest{Protocol: sessionProtocol, Sequence: seq, Action: action})
			if e != nil {
				t.Fatal(e)
			}
			req, e := parseRemoteSessionRequest(raw, seq)
			if e != nil || !remoteSessionRequestAllowed(req.Action, bound, stopped, false, false, controlledIssued, resumed) {
				t.Fatalf("fixed request rejected before its native challenge: action=%s error=%v", action, e)
			}
			switch action {
			case "bind":
				bound = true
			case "stop":
				stopped = true
			case "controlled_resume":
				controlledIssued, resumed = true, true
			}
		})
	}
}

func TestRemoteSessionRequestGateRejectsEarlyPurgeAndRetainsRecovery(t *testing.T) {
	cases := []struct {
		name, action                                        string
		bound, stopped, refused, recovered, issued, resumed bool
		want                                                bool
	}{
		{name: "before-bind", action: "purge_materials", stopped: true, issued: true, resumed: true},
		{name: "before-stop", action: "purge_materials", bound: true, issued: true, resumed: true},
		{name: "before-resume", action: "purge_materials", bound: true, stopped: true},
		{name: "partial-resume", action: "purge_materials", bound: true, stopped: true, issued: true},
		{name: "refused-before-purge", action: "purge_materials", bound: true, stopped: true, refused: true, issued: true, resumed: true},
		{name: "recovery-before-purge", action: "purge_materials", bound: true, stopped: true, recovered: true, issued: true, resumed: true},
		{name: "unknown-action", action: "purge_other_materials", bound: true, stopped: true, issued: true, resumed: true},
		{name: "bind-not-repeated", action: "bind", bound: true},
		{name: "stop-not-repeated", action: "stop", bound: true, stopped: true},
		{name: "controlled-resume-not-repeated", action: "controlled_resume", bound: true, stopped: true, issued: true, resumed: true},
		{name: "runtime-before-resume", action: "check_running", bound: true, stopped: true, issued: true},
		{name: "controlled-resume-without-bind", action: "controlled_resume", stopped: true},
		{name: "controlled-resume-after-stop", action: "controlled_resume", bound: true, stopped: true, want: true},
		{name: "runtime-after-resume", action: "check_running", bound: true, stopped: true, issued: true, resumed: true, want: true},
		{name: "check-after-stop", action: "check", bound: true, stopped: true, want: true},
		{name: "legacy-initial-stop", action: "stop", want: true},
		{name: "known-refusal-keeps-original-restore", action: "restore", bound: true, stopped: true, refused: true, issued: true, want: true},
		{name: "partial-resume-keeps-original-dependent-restore", action: "restore_dependents", bound: true, stopped: true, refused: true, issued: true, want: true},
		{name: "restore-without-original-stop", action: "restore", bound: true},
		{name: "restore-not-repeated", action: "restore", bound: true, stopped: true, refused: true, recovered: true, issued: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := remoteSessionRequestAllowed(c.action, c.bound, c.stopped, c.refused, c.recovered, c.issued, c.resumed)
			if got != c.want {
				t.Fatalf("request eligibility=%v, want %v", got, c.want)
			}
		})
	}
}
