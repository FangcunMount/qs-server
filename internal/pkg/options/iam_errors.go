package options

import "errors"

// IAM 配置相关错误
var (
	ErrIAMGRPCAddressRequired = errors.New("iam.grpc.address is required when IAM gRPC is enabled")
	ErrIAMTLSCAFileRequired   = errors.New("iam.grpc.tls.ca-file is required when mTLS is enabled")
	ErrIAMTLSCertFileRequired = errors.New("iam.grpc.tls.cert-file is required when mTLS is enabled")
	ErrIAMTLSKeyFileRequired  = errors.New("iam.grpc.tls.key-file is required when mTLS is enabled")
	ErrIAMJWTIssuerRequired   = errors.New("iam.jwt.issuer is required when JWKS is enabled")
	ErrIAMJWTAudienceRequired = errors.New("iam.jwt.audience is required when JWKS is enabled")
	ErrIAMJWKSURLRequired     = errors.New("iam.jwks.url is required when JWKS is enabled")
)
