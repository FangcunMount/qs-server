package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	pinyin "github.com/mozillazg/go-pinyin"
)

type generatedClinicianBundle struct {
	staff     StaffConfig
	clinician ClinicianConfig
}

type clinicianNamePageFetcher func(pageURL string) (string, error)

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
	defaultGeneratedClinicianNameSourceURLPattern = "https://www.haodf.com/citiao/jibing-xiaoerduodongzheng/tuijian-doctor.html?p=%d"
	defaultGeneratedClinicianNameSourceReferer    = "https://www.haodf.com/"
	defaultGeneratedClinicianHTTPClient           = &http.Client{Timeout: 20 * time.Second}
	generatedClinicianNameListRegexp              = regexp.MustCompile(`(?s)<ul class="tuijian-list js-tuijian-list".*?</ul>`)
	generatedClinicianNameRegexp                  = regexp.MustCompile(`<span class="name">([^<]+)</span>`)
	generatedClinicianZhNameRegexp                = regexp.MustCompile(`^[\p{Han}·]{2,8}$`)
	generatedClinicianBundleCache                 sync.Map
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
	cacheKey, err := clinicianGeneratorCacheKey(cfg)
	if err != nil {
		return nil, err
	}
	if cached, ok := generatedClinicianBundleCache.Load(cacheKey); ok {
		return cloneGeneratedClinicianBundles(cached.([]generatedClinicianBundle)), nil
	}

	items, err := expandClinicianGeneratorWithFetcher(cfg, fetchGeneratedClinicianNameSourcePage)
	if err != nil {
		return nil, err
	}
	generatedClinicianBundleCache.Store(cacheKey, cloneGeneratedClinicianBundles(items))
	return cloneGeneratedClinicianBundles(items), nil
}

func expandClinicianGeneratorWithFetcher(cfg ClinicianGeneratorConfig, fetcher clinicianNamePageFetcher) ([]generatedClinicianBundle, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("name source fetcher is nil")
	}

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
	password := strings.TrimSpace(cfg.Password)
	if password == "" {
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

	names, err := fetchGeneratedClinicianNames(cfg, fetcher)
	if err != nil {
		return nil, err
	}
	if len(names) < count {
		return nil, fmt.Errorf("name source only returned %d unique doctor names, need %d", len(names), count)
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
		name := names[i]
		emailBase, err := buildGeneratedClinicianEmailLocal(name)
		if err != nil {
			return nil, fmt.Errorf("build email local part for name %q: %w", name, err)
		}
		emailLocal := formatGeneratedClinicianEmailLocal(emailBase, suffix)

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
				Email:    fmt.Sprintf("%s@%s", emailLocal, emailDomain),
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

func clinicianGeneratorCacheKey(cfg ClinicianGeneratorConfig) (string, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal clinician generator config: %w", err)
	}
	sum := sha1.Sum(raw)
	return hex.EncodeToString(sum[:]), nil
}

func cloneGeneratedClinicianBundles(items []generatedClinicianBundle) []generatedClinicianBundle {
	result := make([]generatedClinicianBundle, 0, len(items))
	for _, item := range items {
		cloned := generatedClinicianBundle{
			staff:     item.staff,
			clinician: item.clinician,
		}
		if len(item.staff.Roles) > 0 {
			cloned.staff.Roles = append([]string(nil), item.staff.Roles...)
		}
		result = append(result, cloned)
	}
	return result
}

func fetchGeneratedClinicianNames(cfg ClinicianGeneratorConfig, fetcher clinicianNamePageFetcher) ([]string, error) {
	urlPattern := strings.TrimSpace(cfg.NameSourceURLPattern)
	if urlPattern == "" {
		urlPattern = defaultGeneratedClinicianNameSourceURLPattern
	}
	pages := cfg.NameSourcePages
	if pages <= 0 {
		pages = estimateGeneratedClinicianNamePages(cfg.Count)
	}

	seen := make(map[string]struct{}, cfg.Count)
	names := make([]string, 0, cfg.Count)
	for page := 1; page <= pages; page++ {
		pageURL, err := formatGeneratedClinicianSourceURL(urlPattern, page)
		if err != nil {
			return nil, err
		}
		body, err := fetcher(pageURL)
		if err != nil {
			return nil, fmt.Errorf("fetch clinician names from %s: %w", pageURL, err)
		}
		pageNames, err := parseGeneratedClinicianNames(body)
		if err != nil {
			return nil, fmt.Errorf("parse clinician names from %s: %w", pageURL, err)
		}
		for _, name := range pageNames {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
			if len(names) >= cfg.Count {
				return names, nil
			}
		}
	}
	return names, nil
}

func estimateGeneratedClinicianNamePages(count int) int {
	if count <= 0 {
		return 10
	}
	pages := (count + 14) / 15
	if pages < 10 {
		pages = 10
	}
	return pages
}

func formatGeneratedClinicianSourceURL(pattern string, page int) (string, error) {
	if strings.Contains(pattern, "%d") {
		return fmt.Sprintf(pattern, page), nil
	}

	parsed, err := url.Parse(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid nameSourceUrlPattern %q: %w", pattern, err)
	}
	query := parsed.Query()
	query.Set("p", strconv.Itoa(page))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func fetchGeneratedClinicianNameSourcePage(pageURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", defaultGeneratedClinicianNameSourceReferer)

	resp, err := defaultGeneratedClinicianHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	return string(body), nil
}

func parseGeneratedClinicianNames(body string) ([]string, error) {
	if generatedClinicianSourceBlocked(body) {
		return nil, fmt.Errorf("haodf source returned anti-bot page")
	}

	sections := generatedClinicianNameListRegexp.FindAllString(body, -1)
	if len(sections) == 0 {
		sections = []string{body}
	}

	seen := make(map[string]struct{})
	names := make([]string, 0, 16)
	for _, section := range sections {
		for _, match := range generatedClinicianNameRegexp.FindAllStringSubmatch(section, -1) {
			if len(match) < 2 {
				continue
			}
			name := html.UnescapeString(strings.TrimSpace(match[1]))
			if name == "" || name == "好大夫在线" {
				continue
			}
			if !generatedClinicianZhNameRegexp.MatchString(name) {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no doctor names found in source html")
	}
	return names, nil
}

func generatedClinicianSourceBlocked(body string) bool {
	lower := strings.ToLower(body)
	markers := []string{
		"<title>alipay",
		"disposeaction",
		"security strategy",
		"anti-content",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func buildGeneratedClinicianEmailLocal(name string) (string, error) {
	args := pinyin.NewArgs()
	args.Style = pinyin.Normal
	parts := pinyin.LazyPinyin(name, args)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty pinyin result")
	}

	var builder strings.Builder
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				builder.WriteRune(r)
			}
		}
	}
	if builder.Len() == 0 {
		return "", fmt.Errorf("empty normalized pinyin result")
	}
	return builder.String(), nil
}

func formatGeneratedClinicianEmailLocal(base, suffix string) string {
	return fmt.Sprintf("%s%s", base, suffix)
}
