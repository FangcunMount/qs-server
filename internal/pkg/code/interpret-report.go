package code

// 解读报告错误码
const (
	// ErrInterpretReportNotFound - 404: Interpret report not found.
	ErrInterpretReportNotFound int = iota + 110401

	// ErrInterpretReportAlreadyExists - 400: Interpret report already exists.
	ErrInterpretReportAlreadyExists

	// ErrInterpretReportInvalid - 400: Interpret report is invalid.
	ErrInterpretReportInvalid

	// ErrInterpretReportGenerationFailed - 500: Interpret report generation failed.
	ErrInterpretReportGenerationFailed

	// ErrInterpretItemNotFound - 404: Interpret item not found.
	ErrInterpretItemNotFound

	// ErrInterpretItemInvalid - 400: Interpret item is invalid.
	ErrInterpretItemInvalid
)
