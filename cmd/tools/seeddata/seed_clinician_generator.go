package main

import (
	"fmt"
	"strings"
)

type generatedClinicianBundle struct {
	staff     StaffConfig
	clinician ClinicianConfig
}

type generatedNamePart struct {
	zh string
	py string
}

var (
	defaultGeneratedClinicianDepartments = []string{
		"儿童心理健康科",
		"儿童精神科",
		"神经内科",
		"儿童保健科",
		"精神卫生科",
		"小儿心理卫生科",
		"儿科",
		"儿童心理课",
		"儿童神经科",
		"小儿内科",
		"小儿神经内科",
	}
	defaultGeneratedClinicianTitles = []string{
		"主任医师",
		"副主任医师",
		"主治医师",
	}
	defaultGeneratedClinicianRoles = []string{
		"qs:staff",
	}
	defaultGeneratedClinicianSurnames = []generatedNamePart{
		{zh: "王", py: "wang"},
		{zh: "李", py: "li"},
		{zh: "张", py: "zhang"},
		{zh: "刘", py: "liu"},
		{zh: "陈", py: "chen"},
		{zh: "杨", py: "yang"},
		{zh: "赵", py: "zhao"},
		{zh: "黄", py: "huang"},
		{zh: "周", py: "zhou"},
		{zh: "吴", py: "wu"},
		{zh: "徐", py: "xu"},
		{zh: "孙", py: "sun"},
		{zh: "胡", py: "hu"},
		{zh: "朱", py: "zhu"},
		{zh: "高", py: "gao"},
		{zh: "林", py: "lin"},
		{zh: "何", py: "he"},
		{zh: "郭", py: "guo"},
		{zh: "马", py: "ma"},
		{zh: "罗", py: "luo"},
	}
	defaultGeneratedClinicianGivenNames = []generatedNamePart{
		{zh: "嘉宁", py: "jianing"},
		{zh: "思远", py: "siyuan"},
		{zh: "晨曦", py: "chenxi"},
		{zh: "书瑶", py: "shuyao"},
		{zh: "若琳", py: "ruolin"},
		{zh: "泽宇", py: "zeyu"},
		{zh: "雨桐", py: "yutong"},
		{zh: "子谦", py: "ziqian"},
		{zh: "怡然", py: "yiran"},
		{zh: "景行", py: "jingxing"},
		{zh: "明萱", py: "mingxuan"},
		{zh: "安琪", py: "anqi"},
		{zh: "知远", py: "zhiyuan"},
		{zh: "亦涵", py: "yihan"},
		{zh: "文博", py: "wenbo"},
		{zh: "欣妍", py: "xinyan"},
		{zh: "天佑", py: "tianyou"},
		{zh: "可馨", py: "kexin"},
		{zh: "浩然", py: "haoran"},
		{zh: "语彤", py: "yutong"},
	}
)

func effectiveStaffConfigs(config *SeedConfig) ([]StaffConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("seed config is nil")
	}

	result := make([]StaffConfig, 0, len(config.Staffs))
	result = append(result, config.Staffs...)

	seenKeys := make(map[string]struct{}, len(result))
	for idx, item := range result {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		if _, exists := seenKeys[key]; exists {
			return nil, fmt.Errorf("duplicate staff key %q at index %d", key, idx)
		}
		seenKeys[key] = struct{}{}
	}

	for idx, generator := range config.ClinicianGenerators {
		bundles, err := expandClinicianGenerator(generator)
		if err != nil {
			return nil, fmt.Errorf("invalid clinician generator at index %d: %w", idx, err)
		}
		for _, bundle := range bundles {
			if bundle.staff.Key == "" {
				continue
			}
			if _, exists := seenKeys[bundle.staff.Key]; exists {
				return nil, fmt.Errorf("duplicate generated staff key %q", bundle.staff.Key)
			}
			seenKeys[bundle.staff.Key] = struct{}{}
			result = append(result, bundle.staff)
		}
	}

	return result, nil
}

func effectiveClinicianConfigs(config *SeedConfig) ([]ClinicianConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("seed config is nil")
	}

	result := make([]ClinicianConfig, 0, len(config.Clinicians))
	result = append(result, config.Clinicians...)

	seenKeys := make(map[string]struct{}, len(result))
	for idx, item := range result {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		if _, exists := seenKeys[key]; exists {
			return nil, fmt.Errorf("duplicate clinician key %q at index %d", key, idx)
		}
		seenKeys[key] = struct{}{}
	}

	for idx, generator := range config.ClinicianGenerators {
		bundles, err := expandClinicianGenerator(generator)
		if err != nil {
			return nil, fmt.Errorf("invalid clinician generator at index %d: %w", idx, err)
		}
		for _, bundle := range bundles {
			key := strings.TrimSpace(bundle.clinician.Key)
			if key != "" {
				if _, exists := seenKeys[key]; exists {
					return nil, fmt.Errorf("duplicate generated clinician key %q", key)
				}
				seenKeys[key] = struct{}{}
			}
			result = append(result, bundle.clinician)
		}
	}

	return result, nil
}

func expandClinicianGenerator(cfg ClinicianGeneratorConfig) ([]generatedClinicianBundle, error) {
	count := cfg.Count
	if count <= 0 {
		return nil, fmt.Errorf("count must be greater than 0")
	}

	keyPrefix := strings.TrimSpace(cfg.KeyPrefix)
	if keyPrefix == "" {
		keyPrefix = "generated_clinician"
	}
	staffKeyPrefix := strings.TrimSpace(cfg.StaffKeyPrefix)
	if staffKeyPrefix == "" {
		staffKeyPrefix = "generated_staff"
	}
	employeeCodePrefix := strings.TrimSpace(cfg.EmployeeCodePrefix)
	if employeeCodePrefix == "" {
		employeeCodePrefix = "SEED-VCLN-"
	}
	phonePrefix := strings.TrimSpace(cfg.PhonePrefix)
	if phonePrefix == "" {
		phonePrefix = "+86177"
	}
	emailDomain := strings.TrimSpace(cfg.EmailDomain)
	if emailDomain == "" {
		emailDomain = "fangcunmount.com"
	}
	password := cfg.Password
	if strings.TrimSpace(password) == "" {
		password = "Doctor@123"
	}
	roles := nonEmptyStrings(cfg.StaffRoles)
	if len(roles) == 0 {
		roles = defaultGeneratedClinicianRoles
	}
	clinicianType := strings.TrimSpace(cfg.ClinicianType)
	if clinicianType == "" {
		clinicianType = "doctor"
	}

	startIndex := cfg.StartIndex
	if startIndex <= 0 {
		startIndex = 1
	}

	departments := nonEmptyStrings(cfg.Departments)
	if len(departments) == 0 {
		departments = defaultGeneratedClinicianDepartments
	}
	titles := nonEmptyStrings(cfg.Titles)
	if len(titles) == 0 {
		titles = defaultGeneratedClinicianTitles
	}
	if len(defaultGeneratedClinicianSurnames) == 0 || len(defaultGeneratedClinicianGivenNames) == 0 {
		return nil, fmt.Errorf("generated clinician name pool is empty")
	}

	generateStaff := true
	if cfg.GenerateStaff != nil {
		generateStaff = *cfg.GenerateStaff
	}

	width := 3
	if maxNumber := startIndex + count - 1; maxNumber >= 1000 {
		width = len(fmt.Sprintf("%d", maxNumber))
	}

	items := make([]generatedClinicianBundle, 0, count)
	for i := 0; i < count; i++ {
		seq := startIndex + i
		suffix := fmt.Sprintf("%0*d", width, seq)
		surname := defaultGeneratedClinicianSurnames[i%len(defaultGeneratedClinicianSurnames)]
		given := defaultGeneratedClinicianGivenNames[(i/len(defaultGeneratedClinicianSurnames))%len(defaultGeneratedClinicianGivenNames)]
		name := surname.zh + given.zh
		fullPinyin := surname.py + given.py
		staffKey := ""
		if generateStaff {
			staffKey = fmt.Sprintf("%s_%s", staffKeyPrefix, suffix)
		}

		bundle := generatedClinicianBundle{
			clinician: ClinicianConfig{
				Key:           fmt.Sprintf("%s_%s", keyPrefix, suffix),
				OperatorRef:   staffKey,
				Name:          name,
				Department:    departments[i%len(departments)],
				Title:         titles[i%len(titles)],
				ClinicianType: clinicianType,
				EmployeeCode:  fmt.Sprintf("%s%s", employeeCodePrefix, suffix),
				IsActive:      cfg.IsActive,
			},
		}
		if generateStaff {
			bundle.staff = StaffConfig{
				Key:      staffKey,
				Name:     name,
				Phone:    fmt.Sprintf("%s%08d", phonePrefix, 10000+seq),
				Email:    fmt.Sprintf("%s@%s", fullPinyin, emailDomain),
				Password: password,
				Roles:    append([]string(nil), roles...),
				IsActive: cfg.IsActive,
			}
		} else {
			bundle.clinician.OperatorRef = ""
		}
		items = append(items, bundle)
	}
	return items, nil
}
