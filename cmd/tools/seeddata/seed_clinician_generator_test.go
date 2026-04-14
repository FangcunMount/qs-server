package main

import (
	"reflect"
	"testing"
)

func TestExpandClinicianGenerator(t *testing.T) {
	active := true
	items, err := expandClinicianGenerator(ClinicianGeneratorConfig{
		KeyPrefix:          "generated_clinician",
		StaffKeyPrefix:     "generated_staff",
		EmployeeCodePrefix: "SEED-VCLN-",
		PhonePrefix:        "+86177",
		EmailDomain:        "fangcunmount.com",
		Password:           "Doctor@123",
		StaffRoles:         []string{"qs:staff"},
		Count:              3,
		StartIndex:         7,
		Departments:        []string{"儿童心理健康科", "儿童保健科"},
		Titles:             []string{"主任医师", "副主任医师"},
		ClinicianType:      "doctor",
		IsActive:           &active,
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
	if items[0].clinician.Name == "" || items[0].clinician.Name == "虚拟医师007" {
		t.Fatalf("expected generated real-looking name, got=%s", items[0].clinician.Name)
	}
	if items[0].staff.Email == "" || items[0].staff.Email == "virtualdoctor007@fangcunmount.com" {
		t.Fatalf("expected generated pinyin email, got=%s", items[0].staff.Email)
	}
}

func TestEffectiveClinicianConfigsIncludesGenerators(t *testing.T) {
	active := true
	cfg := &SeedConfig{
		Clinicians: []ClinicianConfig{
			{Key: "real_one", EmployeeCode: "REAL001", Name: "真实医师", ClinicianType: "doctor"},
		},
		ClinicianGenerators: []ClinicianGeneratorConfig{
			{
				KeyPrefix:          "generated_clinician",
				StaffKeyPrefix:     "generated_staff",
				EmployeeCodePrefix: "SEED-VCLN-",
				Count:              2,
				ClinicianType:      "doctor",
				IsActive:           &active,
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
}

func TestEffectiveStaffConfigsIncludesGeneratorStaffs(t *testing.T) {
	active := true
	cfg := &SeedConfig{
		Staffs: []StaffConfig{
			{Key: "real_staff", Name: "真实员工", Phone: "+8618000000001", Password: "Doctor@123", Roles: []string{"qs:staff"}},
		},
		ClinicianGenerators: []ClinicianGeneratorConfig{
			{
				KeyPrefix:      "generated_clinician",
				StaffKeyPrefix: "generated_staff",
				Count:          2,
				ClinicianType:  "doctor",
				IsActive:       &active,
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
	if items[1].Email == "" || items[1].Phone == "" {
		t.Fatalf("expected generated staff contact info, got=%+v", items[1])
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
