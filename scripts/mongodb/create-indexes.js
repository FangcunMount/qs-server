// MongoDB索引创建脚本 - 问卷收集&量表测评系统
// 该脚本为各个集合创建合适的查询索引以优化性能

print('开始创建索引...');

// 切换到questionnaire_scale数据库
db = db.getSiblingDB('questionnaire_scale');

// 为activity_logs集合创建索引
print('创建activity_logs索引...');
db.activity_logs.createIndex({ "type": 1 });
db.activity_logs.createIndex({ "timestamp": 1 });
db.activity_logs.createIndex({ "user_id": 1 });
db.activity_logs.createIndex({ "ip_address": 1 });
db.activity_logs.createIndex({ "type": 1, "timestamp": -1 }); // 复合索引，用于类型+时间查询
db.activity_logs.createIndex({ "timestamp": -1 }, { expireAfterSeconds: 31536000 }); // TTL索引，1年后过期

// 为operation_logs集合创建索引
print('创建operation_logs索引...');
db.operation_logs.createIndex({ "type": 1 });
db.operation_logs.createIndex({ "timestamp": 1 });
db.operation_logs.createIndex({ "operator": 1 });
db.operation_logs.createIndex({ "resource_type": 1 });
db.operation_logs.createIndex({ "resource_id": 1 });
db.operation_logs.createIndex({ "resource_type": 1, "resource_id": 1 }); // 复合索引
db.operation_logs.createIndex({ "operator": 1, "timestamp": -1 }); // 复合索引，用于操作者+时间查询
db.operation_logs.createIndex({ "timestamp": -1 }, { expireAfterSeconds: 31536000 }); // TTL索引，1年后过期

// 为submission_logs集合创建索引
print('创建submission_logs索引...');
db.submission_logs.createIndex({ "type": 1 });
db.submission_logs.createIndex({ "questionnaire_id": 1 });
db.submission_logs.createIndex({ "user_id": 1 });
db.submission_logs.createIndex({ "timestamp": 1 });
db.submission_logs.createIndex({ "questionnaire_id": 1, "timestamp": -1 }); // 复合索引
db.submission_logs.createIndex({ "user_id": 1, "timestamp": -1 }); // 复合索引
db.submission_logs.createIndex({ "type": 1, "questionnaire_id": 1 }); // 复合索引
db.submission_logs.createIndex({ "timestamp": -1 }, { expireAfterSeconds: 94608000 }); // TTL索引，3年后过期

// 为system_configs集合创建索引
print('创建system_configs索引...');
db.system_configs.createIndex({ "key": 1 }, { unique: true }); // 唯一索引
db.system_configs.createIndex({ "updated_at": 1 });
db.system_configs.createIndex({ "updated_by": 1 });

print('基础索引创建完成');

// 创建复合索引用于复杂查询
print('创建复合索引...');

// 活动日志的复合索引 - 用于按用户和时间范围查询
db.activity_logs.createIndex({ 
  "user_id": 1, 
  "type": 1, 
  "timestamp": -1 
}, { 
  name: "user_type_time_idx",
  background: true 
});

// 操作日志的复合索引 - 用于审计查询
db.operation_logs.createIndex({
  "resource_type": 1,
  "action": 1,
  "timestamp": -1
}, {
  name: "audit_query_idx",
  background: true
});

// 提交日志的复合索引 - 用于统计分析
db.submission_logs.createIndex({
  "type": 1,
  "questionnaire_id": 1,
  "timestamp": -1
}, {
  name: "submission_stats_idx",
  background: true
});

print('复合索引创建完成');

// 创建文本索引（用于全文搜索）
print('创建文本索引...');

// 为活动日志详情创建文本索引
db.activity_logs.createIndex({
  "details.message": "text",
  "user_agent": "text"
}, {
  name: "activity_text_idx",
  background: true
});

// 为操作日志创建文本索引
db.operation_logs.createIndex({
  "action": "text",
  "changes": "text"
}, {
  name: "operation_text_idx",
  background: true
});

print('文本索引创建完成');

// 创建地理位置索引（如果需要记录位置信息）
print('创建地理位置索引...');

// 为活动日志添加地理位置索引（如果将来需要记录位置）
// db.activity_logs.createIndex({ "location": "2dsphere" });

print('地理位置索引创建完成');

// 验证索引创建情况
print('');
print('索引创建情况验证:');
print('activity_logs集合索引:');
db.activity_logs.getIndexes().forEach(function(index) {
  print('  - ' + index.name + ': ' + JSON.stringify(index.key));
});

print('operation_logs集合索引:');
db.operation_logs.getIndexes().forEach(function(index) {
  print('  - ' + index.name + ': ' + JSON.stringify(index.key));
});

print('submission_logs集合索引:');
db.submission_logs.getIndexes().forEach(function(index) {
  print('  - ' + index.name + ': ' + JSON.stringify(index.key));
});

print('system_configs集合索引:');
db.system_configs.getIndexes().forEach(function(index) {
  print('  - ' + index.name + ': ' + JSON.stringify(index.key));
});

print('');
print('所有索引创建完成！');

// 记录索引创建操作
db.operation_logs.insertOne({
  type: 'index_creation',
  operator: 'system',
  timestamp: new Date(),
  resource_type: 'database',
  resource_id: 'questionnaire_scale',
  action: 'create_indexes',
  changes: {
    message: 'MongoDB索引创建完成',
    collections: ['activity_logs', 'operation_logs', 'submission_logs', 'system_configs'],
    index_count: {
      activity_logs: 8,
      operation_logs: 9,
      submission_logs: 8,
      system_configs: 3
    }
  }
});

print('索引创建操作已记录到operation_logs'); 