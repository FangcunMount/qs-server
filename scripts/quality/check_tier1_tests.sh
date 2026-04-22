#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
package_list="${repo_root}/scripts/quality/tier1-packages.txt"

base_ref=""
if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
	base_ref="origin/${GITHUB_BASE_REF}"
	git -C "${repo_root}" fetch --no-tags --depth=1 origin "${GITHUB_BASE_REF}" >/dev/null 2>&1 || true
elif git -C "${repo_root}" rev-parse --verify HEAD^ >/dev/null 2>&1; then
	base_ref="HEAD^"
else
	echo "ℹ️ 无法确定对比基线，跳过 Tier 1 测试策略校验。"
	exit 0
fi

merge_base="$(git -C "${repo_root}" merge-base "${base_ref}" HEAD)"
changed_files="$(git -C "${repo_root}" diff --name-only "${merge_base}"...HEAD)"

if [[ -z "${changed_files}" ]]; then
	echo "✅ 未检测到变更文件。"
	exit 0
fi

violations=()
while IFS= read -r package_dir; do
	[[ -z "${package_dir}" ]] && continue

	non_test_changed=0
	test_changed=0

	while IFS= read -r file; do
		[[ -z "${file}" ]] && continue
		case "${file}" in
			"${package_dir}"/*.go)
				if [[ "${file}" == *_test.go ]]; then
					test_changed=1
				else
					non_test_changed=1
				fi
				;;
		esac
	done <<< "${changed_files}"

	if [[ ${non_test_changed} -eq 1 && ${test_changed} -eq 0 ]]; then
		violations+=("${package_dir}")
	fi
done < "${package_list}"

if [[ ${#violations[@]} -gt 0 ]]; then
	echo "❌ Tier 1 包存在非测试 Go 变更但未提交同包测试文件变更:"
	printf ' - %s\n' "${violations[@]}"
	exit 1
fi

echo "✅ Tier 1 包测试策略校验通过。"
