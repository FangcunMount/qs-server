package store

// client 存储客户端实例
var client Factory

// Factory 定义了存储接口
type Factory interface {
	Users() UserStore
	Close() error
}

// Client 返回存储客户端实例
func Client() Factory {
	return client
}

// SetClient 设置存储客户端实例
func SetClient(factory Factory) {
	client = factory
}
