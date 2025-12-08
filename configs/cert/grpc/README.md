# gRPC mTLS 证书目录

## 证书文件说明

此目录存放 QS 作为 IAM gRPC 客户端所需的 mTLS 证书：

```
grpc/
├── ca-chain.crt       # CA 证书链（从 IAM 团队获取）
├── qs-client.crt      # QS 客户端证书（从 IAM 团队获取）
└── qs-client.key      # QS 客户端私钥（从 IAM 团队获取，权限 600）
```

## 获取证书

### 开发环境

联系 IAM 团队，使用测试证书：
1. 从 IAM 项目的 `configs/cert/grpc/` 目录复制测试证书
2. 或运行 `make grpc-cert` 生成

### 生产环境

联系 IAM 团队和运维团队申请正式证书。

## 安全注意事项

1. **私钥权限**：确保 `qs-client.key` 权限为 600
   ```bash
   chmod 600 configs/cert/grpc/qs-client.key
   ```

2. **不入版本库**：证书文件已加入 `.gitignore`，不要提交到代码仓库

3. **Kubernetes 部署**：使用 Secret 管理证书
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: qs-grpc-certs
   type: Opaque
   data:
     ca-chain.crt: <base64>
     qs-client.crt: <base64>
     qs-client.key: <base64>
   ```

## 证书验证

验证证书有效性：
```bash
# 查看证书信息
openssl x509 -in qs-client.crt -text -noout

# 检查证书有效期
openssl x509 -in qs-client.crt -noout -dates

# 验证证书链
openssl verify -CAfile ca-chain.crt qs-client.crt
```
