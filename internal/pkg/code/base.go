package code

import (
"github.com/FangcunMount/component-base/pkg/errors"
)

// Common: basic errors.
// Code must start with 1xxxxx.
const (
// ErrSuccess - 200: OK.
ErrSuccess int = iota + 100001

// ErrUnknown - 500: Internal server error.
ErrUnknown

// ErrBind - 400: Error occurred while binding the request body to the struct.
ErrBind

// ErrValidation - 400: Validation failed.
ErrValidation

// ErrTokenInvalid - 401: Token invalid.
ErrTokenInvalid

// ErrPageNotFound - 404: Page not found.
ErrPageNotFound

// ErrInvalidArgument - 400: Invalid argument.
ErrInvalidArgument

// ErrInvalidMessage - 400: Invalid message.
ErrInvalidMessage
)

// common: database errors.
const (
// ErrDatabase - 500: Database error.
ErrDatabase int = iota + 100101
)

// common: authorization and authentication errors.
const (
// ErrEncrypt - 401: Error occurred while encrypting the user password.
ErrEncrypt int = iota + 100201

// ErrSignatureInvalid - 401: Signature is invalid.
ErrSignatureInvalid

// ErrExpired - 401: Token expired.
ErrExpired

// ErrInvalidAuthHeader - 401: Invalid authorization header.
ErrInvalidAuthHeader

// ErrMissingHeader - 401: The Authorization header was empty.
ErrMissingHeader

// ErrPasswordIncorrect - 401: Password was incorrect.
ErrPasswordIncorrect

// PermissionDenied - 403: Permission denied.
ErrPermissionDenied

// ErrTokenGeneration - 500: Failed to generate token.
ErrTokenGeneration

// ErrInternalServerError - 500: Internal server error.
ErrInternalServerError
)

// common: encode/decode errors.
const (
// ErrEncodingFailed - 500: Encoding failed due to an error with the data.
ErrEncodingFailed int = iota + 100301

// ErrDecodingFailed - 500: Decoding failed due to an error with the data.
ErrDecodingFailed

// ErrInvalidJSON - 500: Data is not valid JSON.
ErrInvalidJSON

// ErrEncodingJSON - 500: JSON data could not be encoded.
ErrEncodingJSON

// ErrDecodingJSON - 500: JSON data could not be decoded.
ErrDecodingJSON

// ErrInvalidYaml - 500: Data is not valid Yaml.
ErrInvalidYaml

// ErrEncodingYaml - 500: Yaml data could not be encoded.
ErrEncodingYaml

// ErrDecodingYaml - 500: Yaml data could not be decoded.
ErrDecodingYaml
)

// common: module errors.
const (
// ErrModuleInitializationFailed - 500: Module initialization failed.
ErrModuleInitializationFailed int = iota + 100401

// ErrModuleNotFound - 404: Module not found.
ErrModuleNotFound
)

func init() {
	// basic errors
	register(ErrSuccess, 200, "OK")
	register(ErrUnknown, 500, "Internal server error")
	register(ErrBind, 400, "Error occurred while binding the request body to the struct")
	register(ErrValidation, 400, "Validation failed")
	register(ErrTokenInvalid, 401, "Token invalid")
	register(ErrPageNotFound, 404, "Page not found")
	register(ErrInvalidArgument, 400, "Invalid argument")
	register(ErrInvalidMessage, 400, "Invalid message")

	// database errors
	register(ErrDatabase, 500, "Database error")

	// authorization and authentication errors
	register(ErrEncrypt, 401, "Error occurred while encrypting the user password")
	register(ErrSignatureInvalid, 401, "Signature is invalid")
	register(ErrExpired, 401, "Token expired")
	register(ErrInvalidAuthHeader, 401, "Invalid authorization header")
	register(ErrMissingHeader, 401, "The Authorization header was empty")
	register(ErrPasswordIncorrect, 401, "Password was incorrect")
	register(ErrPermissionDenied, 403, "Permission denied")
	register(ErrTokenGeneration, 500, "Failed to generate token")
	register(ErrInternalServerError, 500, "Internal server error")

	// encode/decode errors
	register(ErrEncodingFailed, 500, "Encoding failed due to an error with the data")
	register(ErrDecodingFailed, 500, "Decoding failed due to an error with the data")
	register(ErrInvalidJSON, 500, "Data is not valid JSON")
	register(ErrEncodingJSON, 500, "JSON data could not be encoded")
	register(ErrDecodingJSON, 500, "JSON data could not be decoded")
	register(ErrInvalidYaml, 500, "Data is not valid Yaml")
	register(ErrEncodingYaml, 500, "Yaml data could not be encoded")
	register(ErrDecodingYaml, 500, "Yaml data could not be decoded")

	// module errors
	register(ErrModuleInitializationFailed, 500, "Module initialization failed")
	register(ErrModuleNotFound, 404, "Module not found")
}

// register 注册错误码到全局注册表
func register(code int, httpStatus int, message string) {
	errors.MustRegister(&coder{
		code:       code,
		httpStatus: httpStatus,
		message:    message,
	})
}

// coder 实现 errors.Coder 接口
type coder struct {
	code       int
	httpStatus int
	message    string
}

func (c *coder) Code() int {
	return c.code
}

func (c *coder) String() string {
	return c.message
}

func (c *coder) Reference() string {
	return ""
}

func (c *coder) HTTPStatus() int {
	return c.httpStatus
}
