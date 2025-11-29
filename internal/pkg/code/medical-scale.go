package code

// 医学量表错误码 (114xxx).
const (
// ErrMedicalScaleInvalidInput 无效的输入参数
ErrMedicalScaleInvalidInput int = iota + 114001

// ErrMedicalScaleNotFound 医学量表不存在
ErrMedicalScaleNotFound

// ErrMedicalScaleAlreadyExists 医学量表已存在
ErrMedicalScaleAlreadyExists

// ErrMedicalScaleFactorNotFound 因子不存在
ErrMedicalScaleFactorNotFound

// ErrMedicalScaleInvalid 医学量表无效
ErrMedicalScaleInvalid
)

func init() {
	register(ErrMedicalScaleInvalidInput, 400, "Invalid input for medical scale")
	register(ErrMedicalScaleNotFound, 404, "Medical scale not found")
	register(ErrMedicalScaleAlreadyExists, 400, "Medical scale already exists")
	register(ErrMedicalScaleFactorNotFound, 404, "Medical scale factor not found")
	register(ErrMedicalScaleInvalid, 400, "Medical scale is invalid")
}
