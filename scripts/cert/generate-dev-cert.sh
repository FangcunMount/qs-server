#!/bin/bash

# 创建证书目录
mkdir -p configs/cert

# 生成 collection-server 证书
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout configs/cert/collection-server.key \
  -out configs/cert/collection-server.crt \
  -subj "/CN=localhost" \
  -addext "subjectAltName = DNS:localhost,IP:127.0.0.1,IP:0.0.0.0"

# 生成 evaluation-server 证书
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout configs/cert/evaluation-server.key \
  -out configs/cert/evaluation-server.crt \
  -subj "/CN=localhost" \
  -addext "subjectAltName = DNS:localhost,IP:127.0.0.1,IP:0.0.0.0"

# 生成 qs-apiserver 证书
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout configs/cert/qs-apiserver.key \
  -out configs/cert/qs-apiserver.crt \
  -subj "/CN=localhost" \
  -addext "subjectAltName = DNS:localhost,IP:127.0.0.1,IP:0.0.0.0"

echo "开发环境证书生成完成！" 