# questionnaire-scale 问卷收集&量表测评系统

questionnaire and scale system, 支持问卷收集、量表测评、数据分析、数据可视化等功能。

## 功能特性

- 问卷收集: 支持问卷的创建、编辑、发布、删除等功能
- 量表测评: 支持量表的创建、编辑、发布、删除等功能
- 解读报告: 支持解读报告的生成、查看、下载等功能

## 软件架构

### 系统架构

![系统架构](./docs/images/system-architecture.png)

### 系统组件说明

序号|分类|组件|职责
--|--|--|--
1|核心组件|api-server|核心领域服务
2|核心组件|collection-server|问卷收集服务
3|核心组件|evaluation-server|测评解读服务
4|旁路组件|qs-collection-system|问卷小程序
5|旁路组件|qs-operating-system|问卷&量表后台
6|旁路组件|qs-sdk-php|PHP版SDK

#### api-server 核心领域服务(api-server)

#### 职责

- 管理核心聚合根，实现聚合模块：
  - 问卷（Questionnaire）
  - 量表（MedicalScale）
  - 答卷（AnswerSheet）
  - 解读报告（InterpretReport）
- 定义参与角色：
  - 填写人（Writer）
  - 受试者（Testee）
  - 阅读者（Reader）

#### 对外提供的服务

- 向“问卷&量表后台”提供 RESTful API
  - 问卷 CURD
  - 量表 CURD
  - 答卷 查看
  - 解读报告 查看
- 向其他服务提供 RESTful API
  - 获取问卷
  - 获取量表
  - 获取答卷
  - 获取解读报告
  - 创建答卷
  - 创建解读报告
- Redis 发布订阅模型
  - 发布「问卷已发布」事件
  - 发布「量表已更新」事件

#### collection-server 问卷收集服务(collection-server)

#### 职责

- 实现核心功能模块：
  - 校验模块（Validation）
- 核心功能：
  - 问卷缓存预热
  - 问卷缓存更新
  - 答卷校验
  - 答卷保存
  - 答卷、解读报告查看

#### 对外提供的服务

- 向“问卷小程序”提供 RESTful 接口
  - 查看问卷
  - 提交答卷
  - 查看原始问卷
  - 查看解读报告
- Redis 发布订阅模型
  - 发布「答卷已保存」事件

#### 对外依赖的服务

- api-server 的 gRPC 接口
  - 获取问卷
  - 获取答卷
  - 获取解读报告
  - 创建答卷
- Redis 发布订阅模型
  - 订阅「问卷已发布」事件

#### evaluation-server 测评解读服务(evaluation-server)

#### 职责

- 实现核心功能模块：
  - 计算模块（Calculation）
  - 解析模块（Evaluation）
- 核心功能：
  - 量表缓存预热
  - 量表缓存更新
  - 答卷分数计算、保存
  - 解读报告生成、保存

#### 对外提供的服务

- 向“问卷&量表后台”提供 RESTful 接口
  - 查看答卷与解读报告
    - 答卷列表
    - 答卷详情
    - 解读报告详情


#### 对外依赖的服务

- api-server 的 gRPC 接口
  - 获取问卷
  - 获取量表
  - 获取答卷
  - 创建解读报告
- Redis 发布订阅模型
  - 订阅「量表已更新」事件
  - 订阅「答卷已保存」事件

#### qs-collection-system 问卷小程序(qs-collection-system)

#### 职责

- 接入「统一用户」服务
  - 用户注册&登录
  - 孩子信息登记
- 接入 collection-server服务
  - 展示问卷
  - 提交答卷
  - 展示原始答卷
  - 展示解读报告

#### 对外依赖的服务

- 「统一用户」服务
- collection-server 的 RESTful API 
  - 查看问卷
  - 提交答卷
  - 查看原始问卷
  - 查看解读报告

#### qs-operating-system 问卷&量表后台(qs-operating-system)

#### 职责

- 接入「统一用户」服务
  - 登录鉴权
- 接入 api-server服务
  - 提供问卷、量表的管理 
    - 创建
    - 编辑
    - 版本发布
  - 查看答卷与解读报告
    - 答卷列表
    - 答卷详情
    - 解读报告详情

#### 向外依赖的服务

- 「统一用户」服务
- api-server 的 RESTful API
  - 问卷 CURD
  - 量表 CURD
  - 答卷 查看
  - 解读报告 查看

#### qs-sdk-php PHP版SDK(qs-sdk-php)

#### 职责

- 封装 qs-api-server 中部分功能，供其他业务系统调用，封装功能包含：
- 获取问卷列表
- 获取问卷详情
- 获取原始答卷列表
- 获取原始答卷详情
- 获取解读包含列表
- 获取解读报告详情

#### 对外提供的服务

- 向其他业务系统提供代码示例
  - 获取问卷列表
  - 获取问卷详情
  - 获取原始答卷列表
  - 获取原始答卷详情
  - 获取解读包含列表
  - 获取解读报告详情

#### 向外依赖的服务

- api-server RESTful API
  - 问卷列表、详情
  - 答卷列表、详情
  - 解读报告列表、详情

### 分层架构

![分层架构图](./docs/images/layered-architecture.png)

#### 分层架构说明

层级|职责
--|--
外部接口层|接收用户请求（Web/SDK），通过 REST 接口发起操作
应用服务层|承接请求逻辑，完成缓存处理、调用领域服务、事件发布
领域服务层|承载领域聚合操作，定义系统核心行为和业务规则
领域模型层|按 DDD 聚合设计，定义问卷、量表、答卷、报告结构与行为，定义领域服务
存储与基础设施层|提供数据存储与缓存，支撑 Pub/Sub、Redis JSON、持久化等

### 模块设计

#### 聚合根模块（Persisted in MongoDB）

模块名|输入|输出|调用依赖
--|--|--|--
questionnaire|问卷结构、问题、选项表单|问卷文档|-
medical-scale|量表结构、因子设置|量表文档|-
answer-sheet|提交答卷（用户 ID、答案内容）|答卷文档|-questionnaire、-medical-scale
interpret-report|答卷、得分结果、解读规则|解读报告文档|-answer-sheet、-medical-scale


#### 无状态功能模块（Stateless）

模块名|输入|输出|调用依赖
--|--|--|--
validation|校验规则、提交数据|校验结果|-
calculation|运算规则、答题数据|答案得分、因子得分|-
evaluation|解读规则、得分数据|解读文案|-
scoring|解读文案|-|-
