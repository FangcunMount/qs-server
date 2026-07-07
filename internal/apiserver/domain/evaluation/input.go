package evaluation

import evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"

type (
	Answer        = evalinput.Answer
	AnswerSheet   = evalinput.AnswerSheet
	Option        = evalinput.Option
	Question      = evalinput.Question
	Questionnaire = evalinput.Questionnaire
)

var (
	AnswerValueKey = evalinput.AnswerValueKey
	StringSet      = evalinput.StringSet
	AbsInt         = evalinput.AbsInt
)
