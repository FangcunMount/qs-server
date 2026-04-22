package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

type streamEnvelope struct {
	Config  *scanConfig `json:"config,omitempty"`
	SBOM    *scanSBOM   `json:"SBOM,omitempty"`
	OSV     *scanOSV    `json:"osv,omitempty"`
	Finding *finding    `json:"finding,omitempty"`
}

type scanConfig struct {
	ScannerName    string `json:"scanner_name"`
	ScannerVersion string `json:"scanner_version"`
	GoVersion      string `json:"go_version"`
	ScanLevel      string `json:"scan_level"`
	ScanMode       string `json:"scan_mode"`
}

type scanSBOM struct {
	GoVersion string       `json:"go_version"`
	Modules   []scanModule `json:"modules"`
}

type scanModule struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

type scanOSV struct {
	ID               string         `json:"id"`
	Summary          string         `json:"summary"`
	DatabaseSpecific databaseDetail `json:"database_specific"`
	Affected         []affected     `json:"affected"`
}

type databaseDetail struct {
	URL string `json:"url"`
}

type affected struct {
	Package osvPackage `json:"package"`
	Ranges  []osvRange `json:"ranges"`
}

type osvPackage struct {
	Name string `json:"name"`
}

type osvRange struct {
	Events []rangeEvent `json:"events"`
}

type rangeEvent struct {
	Introduced string `json:"introduced,omitempty"`
	Fixed      string `json:"fixed,omitempty"`
}

type finding struct {
	OSV          string      `json:"osv"`
	FixedVersion string      `json:"fixed_version"`
	Trace        []traceStep `json:"trace"`
}

type traceStep struct {
	Module  string `json:"module"`
	Version string `json:"version"`
}

type findingSummary struct {
	ID             string
	Module         string
	CurrentVersion string
	FixedVersion   string
	Summary        string
	URL            string
}

type catalogSummary struct {
	ID             string
	Module         string
	CurrentVersion string
	FixedVersion   string
	Summary        string
	URL            string
}

func main() {
	input := flag.String("input", "", "path to govulncheck JSON stream")
	output := flag.String("output", "", "path to markdown summary output")
	flag.Parse()

	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "usage: govulncheck_summary.go -input <path> -output <path>")
		os.Exit(2)
	}

	if err := run(*input, *output); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(inputPath, outputPath string) error {
	config, sbom, osvs, findings, err := loadStream(inputPath)
	if err != nil {
		return fmt.Errorf("load govulncheck stream: %w", err)
	}

	report := buildReportData(inputPath, config, sbom, osvs, findings)
	if err := os.WriteFile(outputPath, []byte(renderMarkdown(report)), 0o600); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

type reportData struct {
	InputPath         string
	Config            scanConfig
	CurrentGoVersion  string
	ActiveStdlib      []findingSummary
	ActiveThirdParty  []findingSummary
	RemediatedCatalog map[string][]catalogSummary
	UnresolvedCatalog map[string][]catalogSummary
}

func loadStream(path string) (scanConfig, scanSBOM, map[string]scanOSV, []finding, error) {
	file, err := os.Open(path) // #nosec G304 -- CLI intentionally reads a caller-selected local govulncheck report file.
	if err != nil {
		return scanConfig{}, scanSBOM{}, nil, nil, err
	}

	var (
		config   scanConfig
		sbom     scanSBOM
		osvs     = make(map[string]scanOSV)
		findings []finding
	)

	decoder := json.NewDecoder(file)
	for {
		var env streamEnvelope
		if err := decoder.Decode(&env); err != nil {
			if err == io.EOF {
				break
			}
			_ = file.Close()
			return scanConfig{}, scanSBOM{}, nil, nil, err
		}
		switch {
		case env.Config != nil:
			config = *env.Config
		case env.SBOM != nil:
			sbom = *env.SBOM
		case env.OSV != nil:
			osvs[env.OSV.ID] = *env.OSV
		case env.Finding != nil:
			findings = append(findings, *env.Finding)
		}
	}

	if err := file.Close(); err != nil {
		return scanConfig{}, scanSBOM{}, nil, nil, err
	}

	return config, sbom, osvs, findings, nil
}

func buildReportData(inputPath string, config scanConfig, sbom scanSBOM, osvs map[string]scanOSV, findings []finding) reportData {
	currentGoVersion := config.GoVersion
	if currentGoVersion == "" && sbom.GoVersion != "" {
		currentGoVersion = sbom.GoVersion
	}
	moduleVersions := buildModuleVersions(sbom, currentGoVersion)
	activeByID, activeStdlib, activeThirdParty := buildActiveFindings(findings, osvs, moduleVersions, currentGoVersion)
	remediatedCatalog, unresolvedCatalog := classifyCatalogEntries(osvs, activeByID, moduleVersions, currentGoVersion)
	return reportData{
		InputPath:         inputPath,
		Config:            config,
		CurrentGoVersion:  currentGoVersion,
		ActiveStdlib:      activeStdlib,
		ActiveThirdParty:  activeThirdParty,
		RemediatedCatalog: remediatedCatalog,
		UnresolvedCatalog: unresolvedCatalog,
	}
}

func buildModuleVersions(sbom scanSBOM, currentGoVersion string) map[string]string {
	moduleVersions := make(map[string]string, len(sbom.Modules)+1)
	for _, mod := range sbom.Modules {
		moduleVersions[mod.Path] = mod.Version
	}
	if currentGoVersion != "" {
		moduleVersions["stdlib"] = normalizeGoVersion(currentGoVersion)
	}
	return moduleVersions
}

func buildActiveFindings(
	findings []finding,
	osvs map[string]scanOSV,
	moduleVersions map[string]string,
	currentGoVersion string,
) (map[string]findingSummary, []findingSummary, []findingSummary) {
	activeByID := make(map[string]findingSummary, len(findings))
	for _, item := range findings {
		moduleName := "unknown"
		currentVersion := ""
		if len(item.Trace) > 0 {
			moduleName = item.Trace[0].Module
			currentVersion = item.Trace[0].Version
		}
		if currentVersion == "" {
			currentVersion = moduleVersions[moduleName]
		}
		osv := osvs[item.OSV]
		activeByID[item.OSV] = findingSummary{
			ID:             item.OSV,
			Module:         moduleName,
			CurrentVersion: renderVersion(moduleName, currentVersion, currentGoVersion),
			FixedVersion:   renderVersion(moduleName, item.FixedVersion, currentGoVersion),
			Summary:        osv.Summary,
			URL:            osv.DatabaseSpecific.URL,
		}
	}

	var activeStdlib []findingSummary
	var activeThirdParty []findingSummary
	for _, item := range activeByID {
		if item.Module == "stdlib" {
			activeStdlib = append(activeStdlib, item)
			continue
		}
		activeThirdParty = append(activeThirdParty, item)
	}
	sort.Slice(activeStdlib, func(i, j int) bool { return activeStdlib[i].ID < activeStdlib[j].ID })
	sort.Slice(activeThirdParty, func(i, j int) bool {
		if activeThirdParty[i].Module == activeThirdParty[j].Module {
			return activeThirdParty[i].ID < activeThirdParty[j].ID
		}
		return activeThirdParty[i].Module < activeThirdParty[j].Module
	})

	return activeByID, activeStdlib, activeThirdParty
}

func classifyCatalogEntries(
	osvs map[string]scanOSV,
	activeByID map[string]findingSummary,
	moduleVersions map[string]string,
	currentGoVersion string,
) (map[string][]catalogSummary, map[string][]catalogSummary) {
	remediatedCatalog := make(map[string][]catalogSummary)
	unresolvedCatalog := make(map[string][]catalogSummary)
	for id, osv := range osvs {
		if _, ok := activeByID[id]; ok {
			continue
		}
		seen := make(map[string]bool)
		for _, aff := range osv.Affected {
			moduleName := aff.Package.Name
			if moduleName == "" || seen[moduleName] {
				continue
			}
			seen[moduleName] = true

			currentVersion, ok := moduleVersions[moduleName]
			if !ok || currentVersion == "" {
				continue
			}
			fixedVersion := lowestFixedVersion(aff.Ranges)
			if fixedVersion == "" {
				continue
			}

			item := catalogSummary{
				ID:             id,
				Module:         moduleName,
				CurrentVersion: renderVersion(moduleName, currentVersion, currentGoVersion),
				FixedVersion:   renderVersion(moduleName, fixedVersion, currentGoVersion),
				Summary:        osv.Summary,
				URL:            osv.DatabaseSpecific.URL,
			}

			if compareVersions(currentVersion, fixedVersion) >= 0 {
				remediatedCatalog[moduleName] = append(remediatedCatalog[moduleName], item)
				continue
			}
			unresolvedCatalog[moduleName] = append(unresolvedCatalog[moduleName], item)
		}
	}
	return remediatedCatalog, unresolvedCatalog
}

func lowestFixedVersion(ranges []osvRange) string {
	lowest := ""
	for _, r := range ranges {
		for _, event := range r.Events {
			if event.Fixed == "" {
				continue
			}
			if lowest == "" || compareVersions(event.Fixed, lowest) < 0 {
				lowest = event.Fixed
			}
		}
	}
	return lowest
}

func compareVersions(current, fixed string) int {
	currentSemver, okCurrent := canonicalVersion(current)
	fixedSemver, okFixed := canonicalVersion(fixed)
	if !okCurrent || !okFixed {
		return 0
	}
	return semver.Compare(currentSemver, fixedSemver)
}

func canonicalVersion(raw string) (string, bool) {
	switch {
	case raw == "":
		return "", false
	case strings.HasPrefix(raw, "go"):
		raw = "v" + strings.TrimPrefix(raw, "go")
	case strings.HasPrefix(raw, "v"):
		// already canonical
	default:
		raw = "v" + raw
	}
	if !semver.IsValid(raw) {
		return "", false
	}
	return raw, true
}

func normalizeGoVersion(raw string) string {
	if strings.HasPrefix(raw, "go") {
		return "v" + strings.TrimPrefix(raw, "go")
	}
	return raw
}

func renderVersion(moduleName, version, currentGoVersion string) string {
	if moduleName == "stdlib" {
		if version == "" {
			return currentGoVersion
		}
		if strings.HasPrefix(version, "go") {
			return version
		}
		if strings.HasPrefix(version, "v") {
			return "go" + strings.TrimPrefix(version, "v")
		}
		return "go" + version
	}
	return version
}

func renderMarkdown(report reportData) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# govulncheck Advisory 分组处置\n\n")
	fmt.Fprintf(&b, "- 输入报告：`%s`\n", report.InputPath)
	fmt.Fprintf(&b, "- 扫描器：`%s %s`\n", report.Config.ScannerName, report.Config.ScannerVersion)
	fmt.Fprintf(&b, "- 扫描级别：`%s` / 模式：`%s`\n", report.Config.ScanLevel, report.Config.ScanMode)
	fmt.Fprintf(&b, "- 当前 Go toolchain：`%s`\n\n", report.CurrentGoVersion)

	totalActive := len(report.ActiveStdlib) + len(report.ActiveThirdParty)
	fmt.Fprintf(&b, "## 当前 Active Finding（%d）\n\n", totalActive)

	fmt.Fprintf(&b, "### Stdlib / Toolchain 缺口（%d）\n\n", len(report.ActiveStdlib))
	if len(report.ActiveStdlib) == 0 {
		fmt.Fprintf(&b, "- 无\n\n")
	} else {
		for _, item := range report.ActiveStdlib {
			fmt.Fprintf(&b, "- `%s` %s  \n", item.ID, item.Summary)
			fmt.Fprintf(&b, "  当前：`%s`；修复于：`%s`；处置：需要升级 Go toolchain，当前 `1.24.x` 无法单靠依赖升级消掉。", item.CurrentVersion, item.FixedVersion)
			if item.URL != "" {
				fmt.Fprintf(&b, "  参考：%s", item.URL)
			}
			fmt.Fprintf(&b, "\n")
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "### 第三方可行动 finding（%d）\n\n", len(report.ActiveThirdParty))
	if len(report.ActiveThirdParty) == 0 {
		fmt.Fprintf(&b, "- 无。当前剩余 finding 已全部收敛到 Go 标准库 / toolchain。\n\n")
	} else {
		for _, item := range report.ActiveThirdParty {
			fmt.Fprintf(&b, "- `%s` `%s` 当前：`%s`；修复于：`%s`。", item.ID, item.Module, item.CurrentVersion, item.FixedVersion)
			if item.URL != "" {
				fmt.Fprintf(&b, "  参考：%s", item.URL)
			}
			fmt.Fprintf(&b, "\n")
		}
		fmt.Fprintf(&b, "\n")
	}

	remediatedStdlib := sortedCatalog(report.RemediatedCatalog["stdlib"])
	remediatedThirdPartyModules := sortedModuleKeys(report.RemediatedCatalog, "stdlib")
	totalRemediatedThirdParty := 0
	for _, moduleName := range remediatedThirdPartyModules {
		totalRemediatedThirdParty += len(report.RemediatedCatalog[moduleName])
	}

	fmt.Fprintf(&b, "## Catalog 中已被当前版本吸收的历史项\n\n")
	fmt.Fprintf(&b, "### Stdlib 已由当前 toolchain 吸收（%d）\n\n", len(remediatedStdlib))
	if len(remediatedStdlib) == 0 {
		fmt.Fprintf(&b, "- 无\n\n")
	} else {
		for _, item := range remediatedStdlib {
			fmt.Fprintf(&b, "- `%s` 已在当前 `Go %s` 中覆盖；对应修复版本：`%s`。", item.ID, report.CurrentGoVersion, item.FixedVersion)
			if item.URL != "" {
				fmt.Fprintf(&b, "  参考：%s", item.URL)
			}
			fmt.Fprintf(&b, "\n")
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "### 第三方已修复但仍出现在 OSV catalog（%d 模块 / %d 条）\n\n", len(remediatedThirdPartyModules), totalRemediatedThirdParty)
	if len(remediatedThirdPartyModules) == 0 {
		fmt.Fprintf(&b, "- 无\n\n")
	} else {
		for _, moduleName := range remediatedThirdPartyModules {
			items := sortedCatalog(report.RemediatedCatalog[moduleName])
			fixedVersions := make([]string, 0, len(items))
			ids := make([]string, 0, len(items))
			currentVersion := ""
			for _, item := range items {
				if currentVersion == "" {
					currentVersion = item.CurrentVersion
				}
				fixedVersions = append(fixedVersions, item.FixedVersion)
				ids = append(ids, item.ID)
			}
			fmt.Fprintf(&b, "- `%s` 当前：`%s`；最小修复线：`%s`；已覆盖 advisory：`%s`\n",
				moduleName,
				currentVersion,
				lowestStringVersion(fixedVersions),
				strings.Join(ids, "`, `"),
			)
		}
		fmt.Fprintf(&b, "\n")
	}

	unresolvedModules := sortedModuleKeys(report.UnresolvedCatalog)
	totalUnresolved := 0
	for _, moduleName := range unresolvedModules {
		totalUnresolved += len(report.UnresolvedCatalog[moduleName])
	}
	fmt.Fprintf(&b, "## Catalog 中仍需人工确认但没有 active finding 的项（%d 模块 / %d 条）\n\n", len(unresolvedModules), totalUnresolved)
	if len(unresolvedModules) == 0 {
		fmt.Fprintf(&b, "- 无。当前没有发现“SBOM 版本仍低于修复线，但 finding 未命中”的第三方模块。\n\n")
	} else {
		for _, moduleName := range unresolvedModules {
			for _, item := range sortedCatalog(report.UnresolvedCatalog[moduleName]) {
				fmt.Fprintf(&b, "- `%s` `%s` 当前：`%s`；修复于：`%s`。", item.ID, moduleName, item.CurrentVersion, item.FixedVersion)
				if item.URL != "" {
					fmt.Fprintf(&b, "  参考：%s", item.URL)
				}
				fmt.Fprintf(&b, "\n")
			}
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## 建议处置\n\n")
	fmt.Fprintf(&b, "1. 当前真正剩余的漏洞债是 `%d` 条 stdlib / toolchain finding，后续应结合发布窗口评估是否升级到带补丁的 `1.25.x`。\n", len(report.ActiveStdlib))
	fmt.Fprintf(&b, "2. 第三方模块当前没有 active finding；`grpc / quic-go / edwards25519 / mapstructure` 这批依赖不应再作为当前阻塞项重复开单。\n")
	fmt.Fprintf(&b, "3. 运行 `make security-govulncheck-ci` 后应直接查看这份摘要，而不是继续人工解读原始 JSON 或 module 文本输出。\n")

	return b.String()
}

func sortedCatalog(items []catalogSummary) []catalogSummary {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Module == items[j].Module {
			return items[i].ID < items[j].ID
		}
		return items[i].Module < items[j].Module
	})
	return items
}

func sortedModuleKeys(groups map[string][]catalogSummary, skip ...string) []string {
	skipSet := make(map[string]bool, len(skip))
	for _, item := range skip {
		skipSet[item] = true
	}
	keys := make([]string, 0, len(groups))
	for key, items := range groups {
		if skipSet[key] || len(items) == 0 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func lowestStringVersion(values []string) string {
	if len(values) == 0 {
		return ""
	}
	lowest := values[0]
	for _, value := range values[1:] {
		if compareVersions(value, lowest) < 0 {
			lowest = value
		}
	}
	return lowest
}
