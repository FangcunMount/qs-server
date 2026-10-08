//go:build !linux

package compatibilityretirementfence

import (
	"golang.org/x/sys/unix"
	"reflect"
	"regexp"
)

var windowBootIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func windowBootIDValid(id string) bool                { return windowBootIDPattern.MatchString(id) }
func productionWindowOptions() (windowOptions, error) { return windowOptions{}, ErrWindowUnavailable }
func systemWindowClock() (windowClockSample, error)   { return windowClockSample{}, ErrWindowUnavailable }

// Metadata extraction exists only for private filesystem tests on non-Linux.
// Neither it nor a test clock can enable the unavailable production API.
func windowStatTimes(st unix.Stat_t) (int64, int64) {
	v := reflect.ValueOf(st)
	get := func(names ...string) int64 {
		for _, name := range names {
			f := v.FieldByName(name)
			if f.IsValid() {
				sec, nsec := f.FieldByName("Sec"), f.FieldByName("Nsec")
				if sec.IsValid() && nsec.IsValid() {
					return sec.Int()*1000000000 + nsec.Int()
				}
			}
		}
		return 0
	}
	return get("Mtim", "Mtimespec"), get("Ctim", "Ctimespec")
}
