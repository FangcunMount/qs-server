package code

// 解读报告错误码 (115xxx).
const (
// ErrInterpretReportNotFound - 404: Interpret report not found.
ErrInterpretReportNotFound int = iota + 115001

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

func init() {
	register(ErrInterpretReportNotFound, 404, "Interpret report not found")
	register(ErrInterpretReportAlreadyExists, 400, "Interpret report already exists")
	register(ErrInterpretReportInvalid, 400, "Interpret report is invalid")
	register(ErrInterpretReportGenerationFailed, 500, "Interpret report generation failed")
	register(ErrInterpretItemNotFound, 404, "Interpret item not found")
	register(ErrInterpretItemInvalid, 400, "Interpret item is invalid")
}
