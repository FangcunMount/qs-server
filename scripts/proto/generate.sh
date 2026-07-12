#!/bin/bash

# 跨进程 gRPC 契约：源文件与生成代码独立于 apiserver 实现目录。
PROTO_PATH="api/grpc/proto"
GO_OUT_PATH="api/grpc/gen"

mkdir -p "${GO_OUT_PATH}"

generate() {
  local proto_file="$1"
  protoc --proto_path="${PROTO_PATH}" \
         --go_out="${GO_OUT_PATH}" \
         --go_opt=paths=source_relative \
         --go-grpc_out="${GO_OUT_PATH}" \
         --go-grpc_opt=paths=source_relative \
         "${proto_file}"
}

generate "${PROTO_PATH}/answersheet/answersheet.proto"
generate "${PROTO_PATH}/questionnaire/questionnaire.proto"
generate "${PROTO_PATH}/actor/actor.proto"
generate "${PROTO_PATH}/evaluation/evaluation.proto"
generate "${PROTO_PATH}/interpretation/interpretation.proto"
generate "${PROTO_PATH}/internalapi/internal.proto"
generate "${PROTO_PATH}/assessmentmodel/assessment_model_catalog.proto"

echo "Proto files generated successfully!"
