package main

import (
	"strings"
)

// ScaleCategoryMapper 量表分类信息映射器
// 根据 scale.md 文档中的信息，将量表名称映射到分类信息
type ScaleCategoryMapper struct {
	// 量表名称到分类信息的映射
	scaleMap map[string]*ScaleCategoryInfo
}

// ScaleCategoryInfo 量表分类信息
type ScaleCategoryInfo struct {
	Category       string   // 主类
	Stages         []string // 阶段列表
	ApplicableAges []string // 使用年龄列表
	Reporters      []string // 填报人列表
	Tags           []string // 标签列表
}

// NewScaleCategoryMapper 创建分类映射器
func NewScaleCategoryMapper() *ScaleCategoryMapper {
	mapper := &ScaleCategoryMapper{
		scaleMap: make(map[string]*ScaleCategoryInfo),
	}
	mapper.initMapping()
	return mapper
}

// initMapping 初始化映射表（根据 scale.md 文档）
func (m *ScaleCategoryMapper) initMapping() {
	// 根据 scale.md 中的表格数据初始化映射
	m.scaleMap["SNAP-IV量表（18项）"] = &ScaleCategoryInfo{
		Category:       "adhd",
		Stages:         []string{"screening", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent", "teacher"},
		Tags:           []string{"注意缺陷", "多动冲动"},
	}
	m.scaleMap["SNAP-IV量表（26项）"] = &ScaleCategoryInfo{
		Category:       "adhd",
		Stages:         []string{"screening", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent", "teacher"},
		Tags:           []string{"注意缺陷", "多动冲动", "共病/对立外化"},
	}
	m.scaleMap["耶鲁抽动症整体严重程度量表（YGTSS）"] = &ScaleCategoryInfo{
		Category:       "tic",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"clinical", "parent"},
		Tags:           []string{},
	}
	m.scaleMap["耶鲁抽动症整体严重程度量表（家长版）"] = &ScaleCategoryInfo{
		Category:       "tic",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{},
	}
	m.scaleMap["儿童困难问卷（QCD）"] = &ScaleCategoryInfo{
		Category:       "qol",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"school_child"},
		Reporters:      []string{"parent"},
		Tags:           []string{"功能结局/生活质量"},
	}
	m.scaleMap["Conners父母症状问卷（PSQ）"] = &ScaleCategoryInfo{
		Category:       "adhd",
		Stages:         []string{"deep_assessment"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{"多动/冲动", "注意问题"},
	}
	m.scaleMap["Conners教师用量表"] = &ScaleCategoryInfo{
		Category:       "adhd",
		Stages:         []string{"deep_assessment"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"teacher"},
		Tags:           []string{"课堂行为", "注意/冲动"},
	}
	m.scaleMap["Brief-2"] = &ScaleCategoryInfo{
		Category:       "executive",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"preschool", "school_child", "adolescent"},
		Reporters:      []string{"parent", "teacher", "self"},
		Tags:           []string{"情绪调节"},
	}
	m.scaleMap["执行功能行为评定量表（BRIEF）第二版"] = &ScaleCategoryInfo{
		Category:       "executive",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"preschool", "school_child", "adolescent"},
		Reporters:      []string{"parent", "teacher", "self"},
		Tags:           []string{"情绪调节"},
	}
	m.scaleMap["儿童执行功能的行为评定量表--他评问卷"] = &ScaleCategoryInfo{
		Category:       "executive",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent", "teacher"},
		Tags:           []string{"情绪调节"},
	}
	m.scaleMap["SPM"] = &ScaleCategoryInfo{
		Category:       "sensory",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"preschool", "school_child", "adolescent"},
		Reporters:      []string{"parent", "teacher"},
		Tags:           []string{},
	}
	m.scaleMap["儿童感觉统合能力发展评定量表"] = &ScaleCategoryInfo{
		Category:       "sensory",
		Stages:         []string{"deep_assessment"},
		ApplicableAges: []string{"preschool", "school_child"},
		Reporters:      []string{"parent", "teacher"},
		Tags:           []string{},
	}
	m.scaleMap["Weiss功能缺陷量表父母版"] = &ScaleCategoryInfo{
		Category:       "qol",
		Stages:         []string{"outcome", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{"功能结局/生活质量"},
	}
	m.scaleMap["Weiss功能缺陷量表父母版-社会活动"] = &ScaleCategoryInfo{
		Category:       "qol",
		Stages:         []string{"outcome", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{"功能结局/生活质量", "社会能力/同伴"},
	}
	m.scaleMap["焦虑自评量表（SAS）"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "follow_up"},
		ApplicableAges: []string{"adolescent", "adult"},
		Reporters:      []string{"self"},
		Tags:           []string{"焦虑"},
	}
	m.scaleMap["抑郁自评量表（SDS）"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "follow_up"},
		ApplicableAges: []string{"adolescent", "adult"},
		Reporters:      []string{"self"},
		Tags:           []string{"抑郁"},
	}
	m.scaleMap["症状自评量表SCL-90"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "follow_up"},
		ApplicableAges: []string{"adolescent", "adult"},
		Reporters:      []string{"self"},
		Tags:           []string{"广谱心理症状"},
	}
	m.scaleMap["广泛性焦虑障碍量表（GAD-7）"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "follow_up"},
		ApplicableAges: []string{"adolescent", "adult"},
		Reporters:      []string{"self"},
		Tags:           []string{"焦虑"},
	}
	m.scaleMap["Achenbach儿童行为量表"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "deep_assessment", "follow_up"},
		ApplicableAges: []string{"preschool", "school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{"广谱行为情绪"},
	}
	m.scaleMap["Achenbach儿童行为表（CBCL）——一般项目"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "deep_assessment", "follow_up"},
		ApplicableAges: []string{"preschool", "school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{"广谱行为情绪"},
	}
	m.scaleMap["Achenbach儿童行为表（CBCL）——社会能力"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"preschool", "school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{"广谱行为情绪", "社会能力/同伴"},
	}
	m.scaleMap["Achenbach儿童行为表（CBCL）——行为问题"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "deep_assessment", "follow_up"},
		ApplicableAges: []string{"preschool", "school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{"广谱行为情绪"},
	}
	m.scaleMap["范德比尔特ADHD评定量表（父母版）"] = &ScaleCategoryInfo{
		Category:       "adhd",
		Stages:         []string{"deep_assessment", "screening"},
		ApplicableAges: []string{"school_child"},
		Reporters:      []string{"parent"},
		Tags:           []string{"共病/对立外化", "焦虑", "抑郁"},
	}
	m.scaleMap["范德比尔特ADHD评定量表（父母版）+表现"] = &ScaleCategoryInfo{
		Category:       "adhd",
		Stages:         []string{"deep_assessment", "screening"},
		ApplicableAges: []string{"school_child"},
		Reporters:      []string{"parent"},
		Tags:           []string{"共病/对立外化", "焦虑", "抑郁"},
	}
	m.scaleMap["家庭顺应行为量表父母版（FASA）"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"deep_assessment", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{"家庭系统/顺应"},
	}
	m.scaleMap["牛奶相关症状评分(CoMiSS)"] = &ScaleCategoryInfo{
		Category:       "chronic",
		Stages:         []string{"screening"},
		ApplicableAges: []string{"infant"},
		Reporters:      []string{"parent", "clinical"},
		Tags:           []string{"喂养/过敏/消化"},
	}
	m.scaleMap["孤独症谱系障碍筛查性评估（M-CMAT，16-30个月）"] = &ScaleCategoryInfo{
		Category:       "neurodev",
		Stages:         []string{"screening"},
		ApplicableAges: []string{"infant"},
		Reporters:      []string{"parent"},
		Tags:           []string{"ASD筛查/评定"},
	}
	m.scaleMap["注意缺陷多动及攻击评定量表（IOWA）"] = &ScaleCategoryInfo{
		Category:       "adhd",
		Stages:         []string{"follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent", "teacher"},
		Tags:           []string{"攻击/品行"},
	}
	m.scaleMap["压力知觉量表"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "follow_up"},
		ApplicableAges: []string{"adolescent", "adult"},
		Reporters:      []string{"self"},
		Tags:           []string{"压力/应激"},
	}
	m.scaleMap["儿童焦虑性情绪障碍筛查量表（SCARED）"] = &ScaleCategoryInfo{
		Category:       "mental",
		Stages:         []string{"screening", "follow_up"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"self", "parent"},
		Tags:           []string{"焦虑"},
	}
}

// MapScaleCategory 映射量表分类信息
func (m *ScaleCategoryMapper) MapScaleCategory(scaleTitle string) *ScaleCategoryInfo {
	// 精确匹配
	if info, ok := m.scaleMap[scaleTitle]; ok {
		return info
	}

	// 模糊匹配（包含关键词）
	for key, info := range m.scaleMap {
		if strings.Contains(scaleTitle, key) || strings.Contains(key, scaleTitle) {
			return info
		}
	}

	// 默认值
	return &ScaleCategoryInfo{
		Category:       "mental", // 默认心理健康
		Stages:         []string{"screening"},
		ApplicableAges: []string{"school_child", "adolescent"},
		Reporters:      []string{"parent"},
		Tags:           []string{},
	}
}

// parseStages 解析阶段字符串（如 "筛查+症状严重度/随访"）
func parseStages(stageStr string) []string {
	stages := []string{}
	if strings.Contains(stageStr, "筛查") {
		stages = append(stages, "screening")
	}
	if strings.Contains(stageStr, "评估") || strings.Contains(stageStr, "深评") {
		stages = append(stages, "deep_assessment")
	}
	if strings.Contains(stageStr, "随访") {
		stages = append(stages, "follow_up")
	}
	if strings.Contains(stageStr, "结局") || strings.Contains(stageStr, "疗效") {
		stages = append(stages, "outcome")
	}
	if len(stages) == 0 {
		stages = []string{"screening"} // 默认
	}
	return stages
}

// parseApplicableAges 解析使用年龄字符串
func parseApplicableAges(ageStr string) []string {
	ages := []string{}
	if strings.Contains(ageStr, "婴幼儿") || strings.Contains(ageStr, "婴儿") {
		ages = append(ages, "infant")
	}
	if strings.Contains(ageStr, "学龄前") {
		ages = append(ages, "preschool")
	}
	if strings.Contains(ageStr, "学龄") || strings.Contains(ageStr, "儿童") {
		ages = append(ages, "school_child")
	}
	if strings.Contains(ageStr, "青少年") {
		ages = append(ages, "adolescent")
	}
	if strings.Contains(ageStr, "成人") {
		ages = append(ages, "adult")
	}
	if len(ages) == 0 {
		ages = []string{"school_child", "adolescent"} // 默认
	}
	return ages
}

// parseReporters 解析填报人字符串
func parseReporters(reporterStr string) []string {
	reporters := []string{}
	if strings.Contains(reporterStr, "家长") {
		reporters = append(reporters, "parent")
	}
	if strings.Contains(reporterStr, "教师") {
		reporters = append(reporters, "teacher")
	}
	if strings.Contains(reporterStr, "自评") {
		reporters = append(reporters, "self")
	}
	if strings.Contains(reporterStr, "临床") || strings.Contains(reporterStr, "医生") || strings.Contains(reporterStr, "治疗师") {
		reporters = append(reporters, "clinical")
	}
	if len(reporters) == 0 {
		reporters = []string{"parent"} // 默认
	}
	return reporters
}
