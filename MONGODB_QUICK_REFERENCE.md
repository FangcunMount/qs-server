# MongoDB 集合与索引快速参考

## 集合清单

| 集合名称 | 文档数量估计 | 主要查询字段 | 索引状态 | 优先级 |
|---------|-----------|----------|--------|------|
| **questionnaires** | 10K | code, record_role, version | ❌ 缺失 | 🔴 HIGH |
| **answersheets** | 1M | filler_id, questionnaire_code, domain_id | ❌ 缺失 | 🔴 HIGH |
| **answersheet_submit_idempotency** | 10K | idempotency_key, status | ✅ 完整 | - |
| **scales** | 1K | code, questionnaire_code, category | ❌ 缺失 | 🟠 MEDIUM |
| **interpret_reports** | 500K | testee_id, domain_id | ❌ 缺失 | 🔴 HIGH |
| **domain_event_outbox** | 10M | event_id, status, next_attempt_at | ✅ 完整 | - |

## 关键缺失索引 (建议创建)

### Questionnaires 集合

```sql
-- 索引1: 工作版本查询 (核心)
CREATE INDEX idx_code_record_role_deleted 
ON questionnaires(code, record_role, deleted_at);

-- 索引2: 版本查询
CREATE INDEX idx_code_version_record_role_deleted 
ON questionnaires(code, version, record_role, deleted_at);

-- 索引3: 激活版本查询
CREATE INDEX idx_code_active_deleted 
ON questionnaires(code, is_active_published, deleted_at);

-- 索引4: 列表分页排序
CREATE INDEX idx_status_deleted_updated 
ON questionnaires(status, deleted_at, updated_at DESC);
```

### AnswerSheets 集合

```sql
-- 索引1: 按填写者分页查询 (高频)
CREATE INDEX idx_filler_deleted_filled 
ON answersheets(filler_id, deleted_at, filled_at DESC);

-- 索引2: 按问卷分页查询 (高频)
CREATE INDEX idx_question_deleted_filled 
ON answersheets(questionnaire_code, deleted_at, filled_at DESC);

-- 索引3: 主键查询
CREATE INDEX idx_domain_deleted 
ON answersheets(domain_id, deleted_at);
```

### Scales 集合

```sql
-- 索引1: 主键查询
CREATE INDEX idx_code_deleted 
ON scales(code, deleted_at);

-- 索引2: 关联查询
CREATE INDEX idx_question_deleted 
ON scales(questionnaire_code, deleted_at);

-- 索引3: 分类查询
CREATE INDEX idx_category_status_deleted 
ON scales(category, status, deleted_at);
```

### InterpretReports 集合

```sql
-- 索引1: 受试者查询 (高频)
CREATE INDEX idx_testee_deleted_created 
ON interpret_reports(testee_id, deleted_at, created_at DESC);

-- 索引2: 主键查询
CREATE INDEX idx_domain_deleted 
ON interpret_reports(domain_id, deleted_at);
```

## 查询操作热力图

```
频率高度: 🔴 Very High (>100 QPS) | 🟠 High (10-100 QPS) | 🟡 Medium (1-10 QPS)
```

### 按集合分类

| 集合 | 操作 | 频率 | 关键字段 |
|----|------|------|--------|
| questionnaires | FindByCode | 🔴 | code, record_role |
| questionnaires | FindPublishedByCode | 🟠 | code, is_active_published |
| questionnaires | FindBaseList | 🟠 | code, title, status |
| answersheets | FindSummaryListByFiller | 🔴 | filler_id, filled_at |
| answersheets | FindSummaryListByQuestionnaire | 🔴 | questionnaire_code, filled_at |
| answersheets | CountByFiller | 🟠 | filler_id |
| answersheets | CountByQuestionnaire | 🟠 | questionnaire_code |
| scales | FindByCode | 🟠 | code |
| scales | FindByQuestionnaireCode | 🟠 | questionnaire_code |
| scales | FindSummaryList | 🟡 | category, status |
| interpret_reports | FindByTesteeID | 🔴 | testee_id |
| interpret_reports | FindByTesteeIDs | 🟠 | testee_id (IN clause) |
| domain_event_outbox | ClaimDueEvents | 🔴 | status, next_attempt_at |
| domain_event_outbox | MarkEventPublished | 🟠 | event_id |

## 对应的代码位置

```
internal/apiserver/infra/mongo/
├── base.go                           # 基础 Repository (通用CRUD)
├── questionnaire/
│   ├── po.go                         # 数据模型
│   ├── repo.go                       # 📍 查询操作 - 需要添加 ensureIndexes()
│   └── mapper.go
├── answersheet/
│   ├── po.go                         # 数据模型
│   ├── idempotency_po.go
│   ├── repo.go                       # 📍 查询操作
│   ├── durable_submit.go             # ✅ 已有索引定义 (ensureIndexes)
│   └── mapper.go
├── evaluation/
│   ├── po.go                         # 数据模型
│   ├── repo.go                       # 📍 查询操作 - 需要添加 ensureIndexes()
│   └── mapper.go
├── scale/
│   ├── po.go                         # 数据模型
│   ├── repo.go                       # 📍 查询操作 - 需要添加 ensureIndexes()
│   └── mapper.go
└── eventoutbox/
    ├── store.go                      # ✅ 数据模型 + 已有索引 (ensureIndexes)
    └── store_test.go
```

## 执行清单

### 立即执行 (P0 - 本周)

- [ ] **Questionnaires**: 添加 `idx_code_record_role_deleted` 索引
- [ ] **AnswerSheets**: 添加 `idx_filler_deleted_filled` 索引
- [ ] **AnswerSheets**: 添加 `idx_question_deleted_filled` 索引
- [ ] **InterpretReports**: 添加 `idx_testee_deleted_created` 索引
- [ ] 测试索引创建不影响应用启动

### 短期执行 (P1 - 本月)

- [ ] **Scales**: 创建所有推荐索引
- [ ] 在各 Repository 中实现 `ensureIndexes()` 方法
- [ ] 验证查询性能改进

### 持续优化 (P2 - 下月)

- [ ] 监控慢查询日志
- [ ] 检查索引使用率 (`$indexStats`)
- [ ] 清理未使用的索引

## 性能影响评估

```
预期改进:
- 单条查询: 50-100ms → 5-10ms (10-20倍)
- 分页查询: 200-500ms → 20-50ms (5-10倍)
- 排序/聚合: 超时 → 50-100ms (∞倍)
- 并发能力: +300% ~ +500%
```

## 参考文档

- 完整分析: `MONGODB_INDEX_ANALYSIS.md` (本目录)
- MongoDB 官方文档: https://docs.mongodb.com/manual/indexes/
- 索引最佳实践: https://docs.mongodb.com/manual/indexes-best-practices/

---

**生成时间**: 2026-04-27  
**最后更新**: 2026-04-27
