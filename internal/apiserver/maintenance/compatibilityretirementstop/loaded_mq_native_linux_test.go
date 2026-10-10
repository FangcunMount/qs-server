//go:build linux

package compatibilityretirementstop

import (
	"fmt"
	"strings"
	"testing"
)

// Parser boundaries only. No root/Engine/native process authority is injected.
func TestLoadedMQProcMetadataMustNameActualPID1AndStableUID(t *testing.T) {
	uid, e := loadedMQUID([]byte("Name:\tworker\nUid:\t17 17 17 17\nNSpid:\t4321 1\n"))
	if e != nil || uid != 17 {
		t.Fatal("actual namespace status rejected")
	}
	for _, raw := range []string{"Uid:\t17 0 17 17\nNSpid:\t4321 1\n", "Uid:\t17 17 17 17\nNSpid:\t4321 2\n", "Uid:\t17 17 17 17\n", "Uid:\t17 17 17 17\nUid:\t17 17 17 17\nNSpid:\t4321 1\n"} {
		if _, e := loadedMQUID([]byte(raw)); e == nil {
			t.Fatal("ambiguous/reused process status accepted")
		}
	}
	fields := append([]string{"S"}, strings.Fields(strings.Repeat("0 ", 18))...)
	fields = append(fields, "77")
	raw := []byte(fmt.Sprintf("4321 (worker) %s", strings.Join(fields, " ")))
	if ticks, e := loadedMQStart(raw, 4321); e != nil || ticks != 77 {
		t.Fatal("actual ticks rejected")
	}
	if _, e := loadedMQStart(raw, 4322); e == nil {
		t.Fatal("wrong actual hostPID accepted")
	}
	if !validLoadedMQBoot("11111111-2222-3333-4444-555555555555") || validLoadedMQBoot("00000000-0000-0000-0000-000000000000") {
		t.Fatal("boot identity weakened")
	}
}
