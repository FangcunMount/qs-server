#!/bin/bash

# 设置工作目录
PROTO_PATH="internal/apiserver/interface/grpc/proto"
GO_OUT_PATH="internal/apiserver/interface/grpc/proto"

# 确保输出目录存在
mkdir -p ${GO_OUT_PATH}

# 生成 answersheet 服务代码
protoc --proto_path=${PROTO_PATH} \
       --go_out=${GO_OUT_PATH} \
       --go_opt=paths=source_relative \
       --go-grpc_out=${GO_OUT_PATH} \
       --go-grpc_opt=paths=source_relative \
       ${PROTO_PATH}/answersheet/answersheet.proto

# 生成 questionnaire 服务代码
protoc --proto_path=${PROTO_PATH} \
       --go_out=${GO_OUT_PATH} \
       --go_opt=paths=source_relative \
       --go-grpc_out=${GO_OUT_PATH} \
       --go-grpc_opt=paths=source_relative \
       ${PROTO_PATH}/questionnaire/questionnaire.proto

echo "Proto files generated successfully!" 