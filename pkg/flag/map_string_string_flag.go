package flag

import (
	"fmt"
	"sort"
	"strings"
)

// MapStringString 可以设置为命令行格式 `--flag "string=string"`。
// 支持多个标志调用。例如：`--flag "a=foo" --flag "b=bar"`。如果希望这是唯一的类型调用，则应设置 `NoSplit` 为 true。
// 如果 `NoSplit` 设置为 false，则支持单个调用中的多个逗号分隔的键值对。例如：`--flag "a=foo,b=bar"`。
type MapStringString struct {
	Map         *map[string]string
	initialized bool
	NoSplit     bool
}

// NewMapStringString 创建一个 MapStringString 标志解析器
// 接受一个指向 map[string]string 的指针，并返回该 map 的 MapStringString 标志解析器
func NewMapStringString(m *map[string]string) *MapStringString {
	return &MapStringString{Map: m}
}

// NewMapStringStringNoSplit 创建一个 MapStringString 标志解析器
// 接受一个指向 map[string]string 的指针，并设置 `NoSplit` 为 true，然后返回该 map 的 MapStringString 标志解析器
func NewMapStringStringNoSplit(m *map[string]string) *MapStringString {
	return &MapStringString{
		Map:     m,
		NoSplit: true,
	}
}

// String 实现 github.com/spf13/pflag.Value.
func (m *MapStringString) String() string {
	if m == nil || m.Map == nil {
		return ""
	}
	pairs := []string{}
	for k, v := range *m.Map {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// Set 实现 github.com/spf13/pflag.Value.
func (m *MapStringString) Set(value string) error {
	if m.Map == nil {
		return fmt.Errorf("no target (nil pointer to map[string]string)")
	}
	if !m.initialized || *m.Map == nil {
		// clear default values, or allocate if no existing map
		*m.Map = make(map[string]string)
		m.initialized = true
	}

	// account for comma-separated key-value pairs in a single invocation
	if !m.NoSplit {
		for _, s := range strings.Split(value, ",") {
			if len(s) == 0 {
				continue
			}
			arr := strings.SplitN(s, "=", 2)
			if len(arr) != 2 {
				return fmt.Errorf("malformed pair, expect string=string")
			}
			k := strings.TrimSpace(arr[0])
			v := strings.TrimSpace(arr[1])
			(*m.Map)[k] = v
		}
		return nil
	}

	// account for only one key-value pair in a single invocation
	arr := strings.SplitN(value, "=", 2)
	if len(arr) != 2 {
		return fmt.Errorf("malformed pair, expect string=string")
	}
	k := strings.TrimSpace(arr[0])
	v := strings.TrimSpace(arr[1])
	(*m.Map)[k] = v
	return nil
}

// Type 实现 github.com/spf13/pflag.Value.
func (*MapStringString) Type() string {
	return "mapStringString"
}

// Empty 实现 OmitEmpty.
func (m *MapStringString) Empty() bool {
	return len(*m.Map) == 0
}
