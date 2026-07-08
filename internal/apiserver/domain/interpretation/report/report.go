// Package report 负责解释报告 aggregate 和 section structure。
package report

// Section 是 logical report section，独立于测评编码。
type Section struct {
	Title   string
	Content string
	Blocks  []Block
}

// Block 是 renderable report block in section。
type Block struct {
	Kind    string
	Payload any
}
