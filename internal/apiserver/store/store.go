package store

var client Factory

// Factory 定义了qs平台存储接口
type Factory interface {
	Users() UserStore
	Close() error
}

// Client 返回存储客户端实例
func Client() Factory {
	return client
}

// SetClient 设置qs存储客户端
func SetClient(factory Factory) {
	client = factory
}
