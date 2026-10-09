//go:build integration

package compatibilityretirementbackup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type nativeSharedComparison struct {
	Category               string   `json:"category"`
	Equal                  bool     `json:"equal"`
	RawEqual               bool     `json:"raw_equal"`
	ChangedFields          []string `json:"changed_fields"`
	BeforeSHA256           string   `json:"before_sha256"`
	AfterSHA256            string   `json:"after_sha256"`
	NormalizedBeforeSHA256 string   `json:"normalized_before_sha256"`
	NormalizedAfterSHA256  string   `json:"normalized_after_sha256"`
}

// Docker's Container.MountPoints is keyed by destination; GetMountPoints ranges
// that map to produce the inspect Mounts slice. Mount order is display order,
// not identity. Preserve every captured attribute and the exact cardinality;
// reject empty/duplicate destinations instead of collapsing conflicting entries.
// Port binding slices retain their previous exact-order comparison: there is no
// evidence in this leaf that this run's failure was a port-order difference.
func nativeCanonicalSharedMounts(v nativeContainer) (nativeContainer, bool) {
	seen := map[string]bool{}
	v.Mounts = append(v.Mounts[:0:0], v.Mounts...)
	for _, m := range v.Mounts {
		if m.Destination == "" || seen[m.Destination] {
			return v, false
		}
		seen[m.Destination] = true
	}
	sort.Slice(v.Mounts, func(i, j int) bool { return v.Mounts[i].Destination < v.Mounts[j].Destination })
	return v, true
}
func nativeSharedFieldHashes(v nativeContainer) map[string]string {
	raw, _ := json.Marshal(v)
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	hashes := map[string]string{}
	for name, value := range fields {
		hashes[name] = sha(value)
	}
	return hashes
}
func nativeCompareSharedContainer(before, after nativeContainer) nativeSharedComparison {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	r := nativeSharedComparison{Category: "shared_container_attributes_changed", RawEqual: reflect.DeepEqual(before, after), BeforeSHA256: sha(b), AfterSHA256: sha(a), ChangedFields: []string{}}
	bf, af := nativeSharedFieldHashes(before), nativeSharedFieldHashes(after)
	for field, h := range bf {
		if af[field] != h {
			r.ChangedFields = append(r.ChangedFields, field)
		}
	}
	sort.Strings(r.ChangedFields)
	cb, bok := nativeCanonicalSharedMounts(before)
	ca, aok := nativeCanonicalSharedMounts(after)
	if !bok || !aok {
		r.Category = "shared_mount_destination_ambiguous"
		return r
	}
	b, _ = json.Marshal(cb)
	a, _ = json.Marshal(ca)
	r.NormalizedBeforeSHA256 = sha(b)
	r.NormalizedAfterSHA256 = sha(a)
	r.Equal = reflect.DeepEqual(cb, ca)
	if r.Equal {
		r.Category = "shared_mount_order_only"
		if r.RawEqual {
			r.Category = "shared_attributes_exact"
		}
	}
	return r
}

type nativeSharedReadback struct {
	Format     int                    `json:"format_version"`
	Kind       string                 `json:"kind"`
	SourceSHA  string                 `json:"source_sha"`
	Before     nativeContainer        `json:"before"`
	After      *nativeContainer       `json:"after"`
	Comparison nativeSharedComparison `json:"comparison"`
}

func nativeSaveSharedReadback(dir, source string, before nativeContainer, after *nativeContainer) (string, string, error) {
	if !sourcePattern.MatchString(source) || privateDirectory(dir) != nil {
		return "", "", ErrPrivate
	}
	var comparison nativeSharedComparison
	if after != nil {
		comparison = nativeCompareSharedContainer(before, *after)
	} else {
		comparison = nativeCompareSharedContainer(before, before)
		comparison.Category = "shared_readback_unavailable"
		comparison.Equal = false
		comparison.RawEqual = false
		comparison.AfterSHA256 = ""
		comparison.NormalizedAfterSHA256 = ""
		comparison.ChangedFields = []string{"readback"}
	}
	r := nativeSharedReadback{1, "private_closed_shared_container_readback", source, before, after, comparison}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > 2<<20 {
		return "", "", ErrPrivate
	}
	path := filepath.Join(dir, "shared-container-readback-"+primitive.NewObjectID().Hex()+".private.json")
	if writePrivate(path, raw) != nil {
		return "", "", ErrPrivate
	}
	// The complete closed attributes stay only in this O_EXCL/fsynced 0600 file.
	// Report fixed field names and hashes, never mount sources/ports/label values.
	actual, e := os.ReadFile(path)
	st, se := os.Lstat(path)
	if e != nil || se != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || !bytes.Equal(actual, raw) {
		return "", "", ErrPrivate
	}
	return path, sha(raw), nil
}
func nativeSharedTestFixture(t *testing.T) nativeContainer {
	t.Helper()
	var v nativeContainer
	raw := `{"id":"owned-container","name":"/owned","image":"owned-image","labels":{"owner":"private-label-value"},"mounts":[{"Type":"volume","Name":"first","Source":"/private/mount-one","Destination":"/data/db","Driver":"local","Mode":"rw","Propagation":"rprivate","RW":true},{"Type":"volume","Name":"second","Source":"/private/mount-two","Destination":"/data/configdb","Driver":"local","Mode":"rw","Propagation":"rprivate","RW":true}],"ports":{"27017/tcp":[{"HostIP":"127.0.0.1","HostPort":"33317"},{"HostIP":"::1","HostPort":"33317"}]},"requested_ports":{"27017/tcp":[{"HostIP":"127.0.0.1","HostPort":"33317"},{"HostIP":"::1","HostPort":"33317"}]},"network":"bridge","running":true}`
	if json.Unmarshal([]byte(raw), &v) != nil {
		t.Fatal("offline closed-attribute test fixture rejected")
	}
	return v
}
func nativeSharedTestCopy(t *testing.T, v nativeContainer) nativeContainer {
	t.Helper()
	raw, e := json.Marshal(v)
	var out nativeContainer
	if e != nil || json.Unmarshal(raw, &out) != nil {
		t.Fatal("offline closed-attribute copy failed")
	}
	return out
}
func TestNativeSharedReadbackCanonicalMountOrderOnly(t *testing.T) {
	before := nativeSharedTestFixture(t)
	after := nativeSharedTestCopy(t, before)
	after.Mounts[0], after.Mounts[1] = after.Mounts[1], after.Mounts[0]
	originalBefore := nativeSharedTestCopy(t, before)
	originalAfter := nativeSharedTestCopy(t, after)
	r := nativeCompareSharedContainer(before, after)
	if !r.Equal || r.RawEqual || r.Category != "shared_mount_order_only" || !reflect.DeepEqual(r.ChangedFields, []string{"mounts"}) || r.BeforeSHA256 == r.AfterSHA256 || r.NormalizedBeforeSHA256 != r.NormalizedAfterSHA256 {
		t.Fatal("actual record reorder was not narrowly classified")
	}
	if !reflect.DeepEqual(before, originalBefore) || !reflect.DeepEqual(after, originalAfter) {
		t.Fatal("comparison mutated original readbacks")
	}
	for _, kind := range []string{"id", "name", "image", "labels", "network", "running", "mount_type", "mount_name", "mount_source", "mount_destination", "mount_driver", "mount_mode", "mount_propagation", "mount_rw", "mount_removed", "mount_added", "ports_value", "ports_host_ip", "ports_key", "ports_binding_removed", "ports_binding_added", "ports_order", "requested_ports_value", "requested_ports_host_port", "requested_ports_key", "requested_ports_binding_removed", "requested_ports_binding_added", "requested_ports_order"} {
		t.Run(kind, func(t *testing.T) {
			v := nativeSharedTestCopy(t, after)
			switch kind {
			case "id":
				v.ID = "changed"
			case "name":
				v.Name = "changed"
			case "image":
				v.Image = "changed"
			case "labels":
				v.Labels["owner"] = "changed"
			case "network":
				v.Network = "changed"
			case "running":
				v.Running = false
			case "mount_type":
				v.Mounts[0].Type = "bind"
			case "mount_name":
				v.Mounts[0].Name = "changed"
			case "mount_source":
				v.Mounts[0].Source = "changed"
			case "mount_destination":
				v.Mounts[0].Destination = "/changed"
			case "mount_driver":
				v.Mounts[0].Driver = "changed"
			case "mount_mode":
				v.Mounts[0].Mode = "ro"
			case "mount_propagation":
				v.Mounts[0].Propagation = "rshared"
			case "mount_rw":
				v.Mounts[0].RW = false
			case "mount_removed":
				v.Mounts = v.Mounts[:1]
			case "mount_added":
				m := v.Mounts[0]
				m.Destination = "/additional"
				v.Mounts = append(v.Mounts, m)
			case "ports_value":
				v.Ports["27017/tcp"][0].HostPort = "changed"
			case "ports_host_ip":
				v.Ports["27017/tcp"][0].HostIP = "changed"
			case "ports_key":
				v.Ports["changed/tcp"] = v.Ports["27017/tcp"]
				delete(v.Ports, "27017/tcp")
			case "ports_binding_removed":
				v.Ports["27017/tcp"] = v.Ports["27017/tcp"][:1]
			case "ports_binding_added":
				v.Ports["27017/tcp"] = append(v.Ports["27017/tcp"], v.Ports["27017/tcp"][0])
			case "ports_order":
				p := v.Ports["27017/tcp"]
				p[0], p[1] = p[1], p[0]
			case "requested_ports_value":
				v.RequestedPorts["27017/tcp"][0].HostIP = "changed"
			case "requested_ports_host_port":
				v.RequestedPorts["27017/tcp"][0].HostPort = "changed"
			case "requested_ports_key":
				v.RequestedPorts["changed/tcp"] = v.RequestedPorts["27017/tcp"]
				delete(v.RequestedPorts, "27017/tcp")
			case "requested_ports_binding_removed":
				v.RequestedPorts["27017/tcp"] = v.RequestedPorts["27017/tcp"][:1]
			case "requested_ports_binding_added":
				v.RequestedPorts["27017/tcp"] = append(v.RequestedPorts["27017/tcp"], v.RequestedPorts["27017/tcp"][0])
			case "requested_ports_order":
				p := v.RequestedPorts["27017/tcp"]
				p[0], p[1] = p[1], p[0]
			}
			if nativeCompareSharedContainer(before, v).Equal {
				t.Fatal("real closed attribute change accepted")
			}
		})
	}
}
func TestNativeSharedReadbackRejectsAmbiguityAndPreservesNilShape(t *testing.T) {
	for _, kind := range []string{"duplicate_destination", "empty_destination", "nil_empty_mounts", "nil_empty_ports", "nil_empty_bindings"} {
		t.Run(kind, func(t *testing.T) {
			a := nativeSharedTestFixture(t)
			b := nativeSharedTestCopy(t, a)
			switch kind {
			case "duplicate_destination":
				a.Mounts[1].Destination = a.Mounts[0].Destination
				b = nativeSharedTestCopy(t, a)
			case "empty_destination":
				a.Mounts[0].Destination = ""
				b = nativeSharedTestCopy(t, a)
			case "nil_empty_mounts":
				a.Mounts = nil
				b.Mounts = b.Mounts[:0]
			case "nil_empty_ports":
				a.Ports = nil
				b.Ports = map[string][]struct{ HostIP, HostPort string }{}
			case "nil_empty_bindings":
				a.Ports["27017/tcp"] = nil
				b.Ports["27017/tcp"] = b.Ports["27017/tcp"][:0]
			}
			if nativeCompareSharedContainer(a, b).Equal {
				t.Fatal("ambiguous or changed collection shape accepted")
			}
		})
	}
}
func TestNativeSharedReadbackPrivateEvidenceAndBodyFreeProjection(t *testing.T) {
	dir, pathErr := filepath.EvalSymlinks(t.TempDir())
	if pathErr != nil {
		t.Fatal("offline canonical private directory unavailable")
	}
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("offline private fixture chmod failed")
	}
	before := nativeSharedTestFixture(t)
	after := nativeSharedTestCopy(t, before)
	after.Mounts[0], after.Mounts[1] = after.Mounts[1], after.Mounts[0]
	r := nativeCompareSharedContainer(before, after)
	path, h, e := nativeSaveSharedReadback(dir, strings.Repeat("a", 40), before, &after)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(path)
	var stored nativeSharedReadback
	st, se := os.Stat(path)
	if e != nil || se != nil || st.Mode().Perm() != 0600 || sha(raw) != h || json.Unmarshal(raw, &stored) != nil || !reflect.DeepEqual(stored.Before, before) || stored.After == nil || !reflect.DeepEqual(*stored.After, after) {
		t.Fatal("complete private before/after not durably retained")
	}
	safe, _ := json.Marshal(r)
	for _, value := range []string{"private-label-value", "/private/mount-one", "/private/mount-two", "33317", "owned-container", "owned-image"} {
		if bytes.Contains(safe, []byte(value)) {
			t.Fatal("safe comparison leaked a raw closed attribute")
		}
	}
	path, _, e = nativeSaveSharedReadback(dir, strings.Repeat("a", 40), before, nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, e = os.ReadFile(path)
	if e != nil || json.Unmarshal(raw, &stored) != nil || stored.After != nil || stored.Comparison.Equal || stored.Comparison.AfterSHA256 != "" || stored.Comparison.NormalizedAfterSHA256 != "" || stored.Comparison.RawEqual {
		t.Fatal("unknown readback manufactured an observed after")
	}
}
