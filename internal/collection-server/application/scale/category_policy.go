package scale

const (
	categoryADHD               = "adhd"
	categoryTicDisorder        = "td"
	categoryASD                = "asd"
	categoryPressure           = "pressure"
	categorySensoryIntegration = "sii"
	categoryExecutiveFunction  = "efn"
	categoryEmotion            = "emt"
	categorySleep              = "slp"
	categoryPersonality        = "personality"
)

// isOpenScaleCategory 判断 collection BFF 当前允许展示的量表主类。
func isOpenScaleCategory(category string) bool {
	switch category {
	case categoryADHD,
		categoryTicDisorder,
		categoryASD,
		categoryPressure,
		categorySensoryIntegration,
		categoryExecutiveFunction,
		categoryEmotion,
		categorySleep:
		return true
	default:
		return false
	}
}
