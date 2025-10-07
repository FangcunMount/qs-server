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

# 生成 medical-scale 服务代码
protoc --proto_path=${PROTO_PATH} \
       --go_out=${GO_OUT_PATH} \
       --go_opt=paths=source_relative \
       --go-grpc_out=${GO_OUT_PATH} \
       --go-grpc_opt=paths=source_relative \
       ${PROTO_PATH}/medical-scale/medical-scale.proto

# 生成 interpret-report 服务代码
protoc --proto_path=${PROTO_PATH} \
       --go_out=${GO_OUT_PATH} \
       --go_opt=paths=source_relative \
       --go-grpc_out=${GO_OUT_PATH} \
       --go-grpc_opt=paths=source_relative \
       ${PROTO_PATH}/interpret-report/interpret-report.proto

# 生成 user 服务代码（统一的用户模块）
protoc --proto_path=${PROTO_PATH} \
       --go_out=${GO_OUT_PATH} \
       --go_opt=paths=source_relative \
       --go-grpc_out=${GO_OUT_PATH} \
       --go-grpc_opt=paths=source_relative \
       ${PROTO_PATH}/user/user.proto

# 生成 wechat-account 服务代码
protoc --proto_path=${PROTO_PATH} \
       --go_out=${GO_OUT_PATH} \
       --go_opt=paths=source_relative \
       --go-grpc_out=${GO_OUT_PATH} \
       --go-grpc_opt=paths=source_relative \
       ${PROTO_PATH}/wechat/wechat-account.proto

# 生成 role 服务代码
protoc --proto_path=${PROTO_PATH} \
       --go_out=${GO_OUT_PATH} \
       --go_opt=paths=source_relative \
       --go-grpc_out=${GO_OUT_PATH} \
       --go-grpc_opt=paths=source_relative \
       ${PROTO_PATH}/role/role.proto

echo "Proto files generated successfully!" 