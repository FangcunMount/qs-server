package code

// 医学量表错误码
const (
	// ErrMedicalScaleInvalidInput 无效的输入参数
	ErrMedicalScaleInvalidInput int = iota + 110301
	// ErrMedicalScaleNotFound 医学量表不存在
	ErrMedicalScaleNotFound
	// ErrMedicalScaleAlreadyExists 医学量表已存在
	ErrMedicalScaleAlreadyExists
	// ErrMedicalScaleFactorNotFound 因子不存在
	ErrMedicalScaleFactorNotFound
)
