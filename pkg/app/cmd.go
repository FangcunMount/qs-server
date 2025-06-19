package app

import (
	"runtime"
	"strings"
)

// FormatBaseName 格式化基础名称
func FormatBaseName(basename string) string {
	// 根据操作系统，将名称转换为小写，并去除可执行文件后缀
	if runtime.GOOS == "windows" {
		basename = strings.ToLower(basename)
		basename = strings.TrimSuffix(basename, ".exe")
	}

	return basename
}
