package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestParseGeneratedClinicianNames(t *testing.T) {
	html := testGeneratedClinicianHTML("章依文", "曹庆久", "黄艳军")

	names, err := parseGeneratedClinicianNames(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"章依文", "曹庆久", "黄艳军"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("unexpected names: got=%v want=%v", names, want)
	}
}

func TestExpandClinicianGenerator(t *testing.T) {
	active := true
	items, err := expandClinicianGeneratorWithFetcher(ClinicianGeneratorConfig{
		KeyPrefix:          "generated_clinician",
		StaffKeyPrefix:     "generated_staff",
		EmployeeCodePrefix: "SEED-VCLN-",
		PhonePrefix:        "+86177",
		EmailDomain:        "fangcunmount.com",
		Password:           "Doctor@123",
		StaffRoles:         []string{"qs:staff"},
		Count:              3,
		StartIndex:         7,
		NameSourcePages:    1,
		Departments:        []string{"儿童心理健康科", "儿童保健科"},
		Titles:             []string{"主任医师", "副主任医师"},
		ClinicianType:      "doctor",
		IsActive:           &active,
	}, func(_ string) (string, error) {
		return testGeneratedClinicianHTML("章依文", "李锋", "韩颖"), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("unexpected count: got=%d want=3", len(items))
	}

	if items[0].clinician.Key != "generated_clinician_007" || items[0].clinician.EmployeeCode != "SEED-VCLN-007" {
		t.Fatalf("unexpected first generated clinician: %+v", items[0].clinician)
	}
	if items[0].staff.Key != "generated_staff_007" || items[0].staff.Phone != "+8617700010007" {
		t.Fatalf("unexpected generated staff: %+v", items[0].staff)
	}
	if items[1].clinician.Department != "儿童保健科" || items[1].clinician.Title != "副主任医师" {
		t.Fatalf("unexpected round-robin fields: %+v", items[1].clinician)
	}
	if items[2].clinician.Department != "儿童心理健康科" || items[2].clinician.Title != "主任医师" {
		t.Fatalf("unexpected wrapped fields: %+v", items[2].clinician)
	}
	if items[0].clinician.Name != "章依文" {
		t.Fatalf("expected scraped name, got=%s", items[0].clinician.Name)
	}
	if items[0].staff.Email != "zhangyiwen007@fangcunmount.com" {
		t.Fatalf("expected pinyin email, got=%s", items[0].staff.Email)
	}
}

func TestEffectiveClinicianConfigsIncludesGenerators(t *testing.T) {
	active := true
	server := newGeneratedClinicianNameSourceServer("章依文", "李锋")
	defer server.Close()

	cfg := &SeedConfig{
		Clinicians: []ClinicianConfig{
			{Key: "real_one", EmployeeCode: "REAL001", Name: "真实医师", ClinicianType: "doctor"},
		},
		ClinicianGenerators: []ClinicianGeneratorConfig{
			{
				KeyPrefix:            "generated_clinician",
				StaffKeyPrefix:       "generated_staff",
				EmployeeCodePrefix:   "SEED-VCLN-",
				NameSourceURLPattern: server.URL + "/names?p=%d",
				NameSourcePages:      1,
				Count:                2,
				ClinicianType:        "doctor",
				IsActive:             &active,
			},
		},
	}

	items, err := effectiveClinicianConfigs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("unexpected effective clinician count: got=%d want=3", len(items))
	}
	if items[0].Key != "real_one" {
		t.Fatalf("expected explicit clinician first, got=%s", items[0].Key)
	}
	if items[1].Key != "generated_clinician_001" || items[2].Key != "generated_clinician_002" {
		t.Fatalf("unexpected generated clinician keys: %+v %+v", items[1], items[2])
	}
	if items[1].Name != "章依文" || items[2].Name != "李锋" {
		t.Fatalf("unexpected generated clinician names: %+v %+v", items[1], items[2])
	}
}

func TestEffectiveStaffConfigsIncludesGeneratorStaffs(t *testing.T) {
	active := true
	server := newGeneratedClinicianNameSourceServer("章依文", "李锋")
	defer server.Close()

	cfg := &SeedConfig{
		Staffs: []StaffConfig{
			{Key: "real_staff", Name: "真实员工", Phone: "+8618000000001", Password: "Doctor@123", Roles: []string{"qs:staff"}},
		},
		ClinicianGenerators: []ClinicianGeneratorConfig{
			{
				KeyPrefix:            "generated_clinician",
				StaffKeyPrefix:       "generated_staff",
				NameSourceURLPattern: server.URL + "/names?p=%d",
				NameSourcePages:      1,
				Count:                2,
				ClinicianType:        "doctor",
				IsActive:             &active,
			},
		},
	}

	items, err := effectiveStaffConfigs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("unexpected effective staff count: got=%d want=3", len(items))
	}
	if items[1].Key != "generated_staff_001" || items[2].Key != "generated_staff_002" {
		t.Fatalf("unexpected generated staff keys: %+v %+v", items[1], items[2])
	}
	if items[1].Email != "zhangyiwen001@fangcunmount.com" || items[2].Email != "lifeng002@fangcunmount.com" {
		t.Fatalf("expected scraped-name emails, got=%+v %+v", items[1], items[2])
	}
}

func TestClinicianRefsByPrefix(t *testing.T) {
	index := map[string]ClinicianConfig{
		"seed_doctor_002": {Key: "seed_doctor_002"},
		"seed_doctor_001": {Key: "seed_doctor_001"},
		"real_doctor":     {Key: "real_doctor"},
	}

	got := clinicianRefsByPrefix(index, "seed_doctor_")
	want := []string{"seed_doctor_001", "seed_doctor_002"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected refs by prefix: got=%v want=%v", got, want)
	}
}

func TestExpandClinicianGeneratorUsesLocalSnapshot(t *testing.T) {
	active := true
	cfg := ClinicianGeneratorConfig{
		KeyPrefix:            "generated_clinician",
		StaffKeyPrefix:       "generated_staff",
		EmployeeCodePrefix:   "SEED-VCLN-",
		Count:                2,
		StartIndex:           1,
		ClinicianType:        "doctor",
		IsActive:             &active,
		NameSourceURLPattern: "://invalid-if-fetch-is-used",
	}

	cacheKey, err := clinicianGeneratorCacheKey(cfg)
	if err != nil {
		t.Fatalf("cache key: %v", err)
	}

	snapshotDir := t.TempDir()
	oldResolver := generatedClinicianSnapshotDirResolver
	generatedClinicianSnapshotDirResolver = func() (string, error) {
		return snapshotDir, nil
	}
	t.Cleanup(func() {
		generatedClinicianSnapshotDirResolver = oldResolver
		generatedClinicianBundleCache = sync.Map{}
	})
	generatedClinicianBundleCache = sync.Map{}

	if err := saveGeneratedClinicianNamesSnapshot(cacheKey, []string{"章依文", "李锋"}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	items, err := expandClinicianGenerator(cfg)
	if err != nil {
		t.Fatalf("expand generator using snapshot: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("unexpected item count: got=%d want=2", len(items))
	}
	if items[0].clinician.Name != "章依文" || items[1].clinician.Name != "李锋" {
		t.Fatalf("unexpected names from snapshot: got=%q,%q", items[0].clinician.Name, items[1].clinician.Name)
	}
}

func testGeneratedClinicianHTML(names ...string) string {
	var builder strings.Builder
	builder.WriteString(`<html><body><ul class="tuijian-list js-tuijian-list">`)
	for _, name := range names {
		builder.WriteString(fmt.Sprintf(`<li><div class="left"><span class="name">%s</span></div></li>`, name))
	}
	builder.WriteString(`</ul></body></html>`)
	return builder.String()
}

func newGeneratedClinicianNameSourceServer(names ...string) *httptest.Server {
	html := testGeneratedClinicianHTML(names...)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
}
