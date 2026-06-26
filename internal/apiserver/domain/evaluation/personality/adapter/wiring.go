package adapter

import (
	bigfivepkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/bigfive"
	mbtipkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/mbti"
	sbtipkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/sbti"
)

func mbtiAdapter() ModelAdapter {
	return mbtipkg.Adapter{}
}

func sbtiAdapter() ModelAdapter {
	return sbtipkg.Adapter{}
}

func bigfiveAdapter() ModelAdapter {
	return bigfivepkg.Adapter{}
}
