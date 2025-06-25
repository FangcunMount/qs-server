// MongoDB初始化脚本 - 问卷收集&量表测评系统
// 该脚本在MongoDB容器首次启动时自动执行

print('开始初始化问卷收集&量表测评系统数据库...');

// 切换到questionnaire_scale数据库
db = db.getSiblingDB('questionnaire_scale');

// 创建应用用户
db.createUser({
  user: 'qs_app_user',
  pwd: 'qs_app_password_2024',
  roles: [
    {
      role: 'readWrite',
      db: 'questionnaire_scale'
    },
    {
      role: 'dbAdmin',
      db: 'questionnaire_scale'
    }
  ]
});

print('创建应用用户成功: qs_app_user');

// 创建只读用户（用于分析和报告）
db.createUser({
  user: 'qs_readonly_user',
  pwd: 'qs_readonly_password_2024',
  roles: [
    {
      role: 'read',
      db: 'questionnaire_scale'
    }
  ]
});

print('创建只读用户成功: qs_readonly_user');

// 创建基础集合并插入示例数据
print('创建基础集合...');

// 创建活动日志集合
db.createCollection('activity_logs', {
  validator: {
    $jsonSchema: {
      bsonType: 'object',
      required: ['type', 'timestamp', 'ip_address'],
      properties: {
        type: {
          bsonType: 'string',
          description: '活动类型，如: user_activity, questionnaire_submission等'
        },
        user_id: {
          bsonType: 'string',
          description: '用户ID（可选）'
        },
        timestamp: {
          bsonType: 'date',
          description: '活动时间戳'
        },
        ip_address: {
          bsonType: 'string',
          description: 'IP地址'
        },
        user_agent: {
          bsonType: 'string',
          description: '用户代理字符串'
        },
        details: {
          bsonType: 'object',
          description: '活动详细信息'
        }
      }
    }
  }
});

// 创建操作日志集合
db.createCollection('operation_logs', {
  validator: {
    $jsonSchema: {
      bsonType: 'object',
      required: ['type', 'timestamp'],
      properties: {
        type: {
          bsonType: 'string',
          description: '操作类型，如: user_creation, questionnaire_update等'
        },
        operator: {
          bsonType: 'string',
          description: '操作者'
        },
        timestamp: {
          bsonType: 'date',
          description: '操作时间戳'
        },
        resource_type: {
          bsonType: 'string',
          description: '资源类型'
        },
        resource_id: {
          bsonType: 'string',
          description: '资源ID'
        },
        action: {
          bsonType: 'string',
          description: '操作动作'
        },
        changes: {
          bsonType: 'object',
          description: '变更内容'
        }
      }
    }
  }
});

// 创建提交日志集合
db.createCollection('submission_logs', {
  validator: {
    $jsonSchema: {
      bsonType: 'object',
      required: ['type', 'questionnaire_id', 'timestamp'],
      properties: {
        type: {
          bsonType: 'string',
          enum: ['questionnaire_submission', 'scale_submission'],
          description: '提交类型'
        },
        questionnaire_id: {
          bsonType: 'string',
          description: '问卷ID'
        },
        user_id: {
          bsonType: 'string',
          description: '用户ID'
        },
        timestamp: {
          bsonType: 'date',
          description: '提交时间戳'
        },
        answers: {
          bsonType: 'object',
          description: '答案内容'
        },
        metadata: {
          bsonType: 'object',
          description: '元数据'
        }
      }
    }
  }
});

// 创建系统配置集合
db.createCollection('system_configs', {
  validator: {
    $jsonSchema: {
      bsonType: 'object',
      required: ['key', 'value', 'updated_at'],
      properties: {
        key: {
          bsonType: 'string',
          description: '配置键'
        },
        value: {
          description: '配置值'
        },
        description: {
          bsonType: 'string',
          description: '配置描述'
        },
        updated_at: {
          bsonType: 'date',
          description: '更新时间'
        },
        updated_by: {
          bsonType: 'string',
          description: '更新者'
        }
      }
    }
  }
});

print('基础集合创建完成');

// 插入初始系统配置
db.system_configs.insertMany([
  {
    key: 'system_initialized',
    value: true,
    description: '系统是否已初始化',
    updated_at: new Date(),
    updated_by: 'system'
  },
  {
    key: 'mongodb_version',
    value: '7.0',
    description: 'MongoDB版本',
    updated_at: new Date(),
    updated_by: 'system'
  },
  {
    key: 'data_retention_days',
    value: 365,
    description: '数据保留天数',
    updated_at: new Date(),
    updated_by: 'system'
  }
]);

print('初始系统配置插入完成');

// 插入示例活动日志
db.activity_logs.insertOne({
  type: 'system_initialization',
  timestamp: new Date(),
  ip_address: '127.0.0.1',
  user_agent: 'MongoDB-Init-Script',
  details: {
    message: 'MongoDB数据库初始化完成',
    version: '1.0.0'
  }
});

print('示例数据插入完成');

print('问卷收集&量表测评系统数据库初始化完成！');
print('');
print('数据库信息:');
print('- 数据库名: questionnaire_scale');
print('- 应用用户: qs_app_user');
print('- 只读用户: qs_readonly_user');
print('- 创建的集合: activity_logs, operation_logs, submission_logs, system_configs');
print(''); 