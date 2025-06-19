package flag

import (
	"fmt"
	"strconv"
)

// Tristate 是一个标志，兼容 flags 和 pflags，并跟踪是否提供了值
// 注：也就是一个布尔值，但是可以设置为未设置、真、假
type Tristate int

const (
	Unset Tristate = iota // 0
	True
	False
)

// Default 设置默认值
func (f *Tristate) Default(value bool) {
	*f = triFromBool(value)
}

// String 返回标志的值
func (f Tristate) String() string {
	b := boolFromTri(f)
	return fmt.Sprintf("%t", b)
}

// Value 返回标志的值
func (f Tristate) Value() bool {
	b := boolFromTri(f)
	return b
}

// Set 设置标志的值
func (f *Tristate) Set(value string) error {
	boolVal, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}

	*f = triFromBool(boolVal)
	return nil
}

// Provided 返回标志是否被提供
func (f Tristate) Provided() bool {
	return f != Unset
}

// Type 返回标志的类型
func (f *Tristate) Type() string {
	return "tristate"
}

// boolFromTri 将 Tristate 转换为 bool
func boolFromTri(t Tristate) bool {
	if t == True {
		return true
	} else {
		return false
	}
}

// triFromBool 将 bool 转换为 Tristate
func triFromBool(b bool) Tristate {
	if b {
		return True
	} else {
		return False
	}
}
