// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package apiserver

import (
	"net"

	"google.golang.org/grpc"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// grpcAPIServer 定义了 grpc api 服务器
type grpcAPIServer struct {
	*grpc.Server
	address string
}

// Run 运行 grpc api 服务器
func (s *grpcAPIServer) Run() {
	listen, err := net.Listen("tcp", s.address)
	if err != nil {
		log.Fatalf("failed to listen: %s", err.Error())
	}

	go func() {
		if err := s.Serve(listen); err != nil {
			log.Fatalf("failed to start grpc server: %s", err.Error())
		}
	}()

	log.Infof("start grpc server at %s", s.address)
}

// Close 关闭 grpc api 服务器
func (s *grpcAPIServer) Close() {
	s.GracefulStop()
	log.Infof("GRPC server on %s stopped", s.address)
}
