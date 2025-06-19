package flag

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/pflag"
)

// NamedFlagSets 存储按调用 FlagSet 顺序命名的标志集
type NamedFlagSets struct {
	// Order 按顺序存储标志集名称的列表
	Order []string
	// FlagSets 按名称存储标志集
	FlagSets map[string]*pflag.FlagSet
}

// FlagSet 设置标识符集
// 注：也就是给命令行参数进行分组
func (nfs *NamedFlagSets) FlagSet(name string) *pflag.FlagSet {
	// 如果标志集为空，则初始化标志集
	if nfs.FlagSets == nil {
		nfs.FlagSets = map[string]*pflag.FlagSet{}
	}
	// 如果标志集不存在，则初始化标志集
	if _, ok := nfs.FlagSets[name]; !ok {
		// 创建新的标志集
		nfs.FlagSets[name] = pflag.NewFlagSet(name, pflag.ExitOnError)
		// 将标志集名称添加到列表中
		nfs.Order = append(nfs.Order, name)
	}

	// 返回标志集
	return nfs.FlagSets[name]
}

// PrintSections 打印给定的标志集名称，并按最大列数进行分组
// 如果 cols 为零，则不进行换行
func PrintSections(w io.Writer, fss NamedFlagSets, cols int) {
	for _, name := range fss.Order {
		fs := fss.FlagSets[name]
		if !fs.HasFlags() {
			continue
		}

		wideFS := pflag.NewFlagSet("", pflag.ExitOnError)
		wideFS.AddFlagSet(fs)

		var zzz string
		if cols > 24 {
			zzz = strings.Repeat("z", cols-24)
			wideFS.Int(zzz, 0, strings.Repeat("z", cols-24))
		}

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "\n%s flags:\n\n%s", strings.ToUpper(name[:1])+name[1:], wideFS.FlagUsagesWrapped(cols))

		if cols > 24 {
			i := strings.Index(buf.String(), zzz)
			lines := strings.Split(buf.String()[:i], "\n")
			fmt.Fprint(w, strings.Join(lines[:len(lines)-1], "\n"))
			fmt.Fprintln(w)
		} else {
			fmt.Fprint(w, buf.String())
		}
	}
}
