package application_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const apiserverImportBase = "github.com/FangcunMount/qs-server/internal/apiserver/"

// forbiddenModuleEdges 表达《docs/02-业务模块/01-模块边界与依赖关系》中禁止的反向依赖方向。
// 仅扫描 domain/application/infra 生产代码；组合根 container/modules/* 允许跨模块装配，故不在范围内。
var forbiddenModuleEdges = []struct{ from, to string }{
	{"survey", "evaluation"},
	{"survey", "interpretation"},
	{"modelcatalog", "evaluation"},
	{"evaluation", "statistics"},
	{"interpretation", "statistics"},
}

// forbiddenDepAllowlist 记录当前已知的越界依赖（生产文件相对路径 -> 原因）。
//
// 约定：
//   - 标 TODO 的条目是待清理的真实越界（对应重构阶段 2/3），消除后必须删除本条目；
//   - 标 accepted 的条目是设计上可接受的依赖（例如只消费事件类型常量）。
//
// 当某条目对应的越界已被消除时，测试会报「allowlist 条目已失效」以提醒收紧护栏。
var forbiddenDepAllowlist = map[string]string{
	"internal/apiserver/application/statistics/journey_router.go": "accepted: 仅消费 evaluation 事件类型常量(domain/evaluation/assessment)",
}

// TestForbiddenCrossModuleImports 守护模块间禁止的反向依赖方向。
func TestForbiddenCrossModuleImports(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	violated := map[string]bool{}

	record := func(rel, msg string) {
		if reason, ok := forbiddenDepAllowlist[rel]; ok {
			violated[rel] = true
			t.Logf("已知越界(allowlisted): %s (%s)", rel, reason)
			return
		}
		t.Errorf("%s", msg)
	}

	scanEdge := func(from, to string) {
		targets := moduleImportPrefixes(to)
		for _, dir := range existingModuleDirs(root, moduleLayerDirs(from)) {
			scanGoImports(t, dir, func(path, importPath string) {
				if strings.HasSuffix(path, "_test.go") {
					return
				}
				for _, tp := range targets {
					if importPath == tp || strings.HasPrefix(importPath, tp+"/") {
						rel := filepath.ToSlash(mustRel(t, root, path))
						record(rel, fmt.Sprintf("%s imports %s; 禁止 %s -> %s 反向依赖", rel, importPath, from, to))
						return
					}
				}
			})
		}
	}

	for _, e := range forbiddenModuleEdges {
		scanEdge(e.from, e.to)
	}

	// 规则 6：statistics 是读侧投影，不得反向依赖核心写模型(domain/application)。infra 读侧复用另议，不在此扫描。
	var writeModelTargets []string
	for _, m := range []string{"survey", "evaluation", "interpretation", "plan"} {
		writeModelTargets = append(writeModelTargets,
			apiserverImportBase+"domain/"+m,
			apiserverImportBase+"application/"+m,
		)
	}
	statisticsDirs := []string{
		filepath.Join("internal", "apiserver", "domain", "statistics"),
		filepath.Join("internal", "apiserver", "application", "statistics"),
	}
	for _, dir := range existingModuleDirs(root, statisticsDirs) {
		scanGoImports(t, dir, func(path, importPath string) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			for _, tp := range writeModelTargets {
				if importPath == tp || strings.HasPrefix(importPath, tp+"/") {
					rel := filepath.ToSlash(mustRel(t, root, path))
					record(rel, fmt.Sprintf("%s imports %s; statistics 不应依赖核心写模型", rel, importPath))
					return
				}
			}
		})
	}

	// 收紧机制：allowlist 中已失效(不再越界)的条目必须删除。
	for rel := range forbiddenDepAllowlist {
		if !violated[rel] {
			t.Errorf("allowlist 条目已失效，请删除: %s", rel)
		}
	}
}

// moduleLayerDirs 返回某业务模块在各层的生产代码目录（相对仓库根）。
func moduleLayerDirs(m string) []string {
	return []string{
		filepath.Join("internal", "apiserver", "domain", m),
		filepath.Join("internal", "apiserver", "application", m),
		filepath.Join("internal", "apiserver", "infra", m),
		filepath.Join("internal", "apiserver", "infra", "mongo", m),
		filepath.Join("internal", "apiserver", "infra", "mysql", m),
	}
}

// moduleImportPrefixes 返回某业务模块各层的 import 路径前缀，用于判定目标依赖。
func moduleImportPrefixes(m string) []string {
	return []string{
		apiserverImportBase + "domain/" + m,
		apiserverImportBase + "application/" + m,
		apiserverImportBase + "infra/" + m,
		apiserverImportBase + "infra/mongo/" + m,
		apiserverImportBase + "infra/mysql/" + m,
	}
}

// existingModuleDirs 过滤出真实存在的目录，返回绝对路径。
func existingModuleDirs(root string, rels []string) []string {
	var out []string
	for _, r := range rels {
		abs := filepath.Join(root, r)
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			out = append(out, abs)
		}
	}
	return out
}
