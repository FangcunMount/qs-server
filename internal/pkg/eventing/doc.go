// Package eventing 事件工程共享入口点
//
// 其子包分离产品事件目录、传输运行时、有界可观察性标签和跨进程线缆合同。
// 进程级生命周期和基础设施所有权保留在 apiserver 和 worker EventSubsystems 中，
// 而不是共享此包。
package eventing
