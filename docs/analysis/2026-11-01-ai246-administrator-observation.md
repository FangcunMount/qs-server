# P. 代码分析报告

## 分析目标与范围

主目标review：补齐AI246数据库执行审查DB07。复用既有管理员Secret和可信ServerA SSH运输，仅增加固定AI库的只读观测，不扩展QS原14项活动观察/clean100就绪协议，不改用户、角色、授权或生产数据库。数据/恢复责任人均为shujie。

## 入口与调用路径、当前职责与依赖

`db-ops.yml operation=ai-activity-admin / database=mysql`先在checkout前要求`governance_confirm`精确等于当次源码、backup_name为空。独立`--ai-activity`运输只接受五个固定普通文件；凭据仅stdin传输，0600临时信封在连接前删除，仍以原sudo接收器验证ServerA/amd64/CA。新Go入口只允许MYSQL_DATABASE=ai，单连接，45秒总预算、5秒单查询/2秒SELECT服务端限时，多语句禁用。

独立目标hash为`83cd3186bd7de27e6a5b5aa0effbb3c5673274f65cd5d4635f36fed7304a5463`；与既有QS route hash共同经原AI数据库只读身份复核。AI原uuid+database JSON身份为`04f119402a8b7704f84df45bd3e3fb182eaf1ea05febe7fbad38f66c76740fc3`，Python运输成功回执还独立绑定该身份。两个编码不混为同一hash。

## 行为 / 契约 / 不变量边界

独立AI只读头只接受完整43或完整44。43需要目标存在，记录真实行数及SHOW CREATE摘要；非空也按真实数观察，不授予删除资格。44需要目标缺席，目标行数/DDL字段省略，不冒充0行。43共24查询、44共22查询。

要求当前全局SELECT/SHOW VIEW/TRIGGER/EVENT且无partial revoke，验证MDL instrument已启用及当前PFS thread映射。观察14个计数：AI会话/活动会话、事务/修改事务、AI已授予/待定MDL、全局待定MDL、AI启用EVENT/trigger/routine/view、目标全部及外schema入向FK、视图依赖。只输出计数/版本/摘要/严格状态；不输出SQL INFO、用户名/host/UUID/凭据、角色名或例程/业务正文。视图依赖可见性依赖对象权限，因此不能只用PROCESS当全局依赖资格；依据[MySQL VIEW_TABLE_USAGE](https://dev.mysql.com/doc/mysql-infoschema-excerpt/8.0/en/information-schema-view-table-usage-table.html)。

结果始终read_only=true、freeze_verified=false、observations_atomic=false。非零按原值保留；任一权限/身份/布局/计时/释放失败丢弃全部measurements，不把未知写成0，不签无写者、无在途DDL或停写证明。routine/view计数也不证明动态SQL/人工入口不存在，窗口仍需责任人的完整writer审查。

## 测试与可观测性现状

本地Go（含race）覆盖精确43/44、非零保留、1142丢弃部分结果、错误目标在grant前拒绝；Python覆盖严格schema、44不造零、源码/operation隔离、凭据只走stdin。原运输27项回归继续运行。原生CI独立owned MySQL8.0.36验证真实queued DDL/MDL正对照、权限不足、原二进制记录与ledger不变、实际视图依赖及44物理缺席；必须实际RUN/PASS、零SKIP并导出独立JSON。当前源码提交前本地只收集/编译原生用例，真实结果由CI/生产只读回执另行绑定。

## 分析指标与判定

| 指标 | 判定与依据 |
| --- | --- |
| 目标与入口 | 绿：固定AI库、独立operation/flag/五文件及双身份摘要 |
| 原QS兼容 | 绿：原QS模式/账本/14查询不改，原运输回归保留 |
| 只读范围 | 绿：生产入口只调用SELECT/SHOW；无DML/DDL/GRANT/KILL/role activation |
| 未知处理 | 绿：全部计数与释放完整方可complete，失败删除部分measurements |
| 真实观察资格 | 黄：原生positive control及实际管理员路径须独立通过 |
| 窗口冻结 | 黄：顺序观察不认证停写，正式窗口仍待独立排空/保护/批准 |

## 主要风险点、初步发现或假设

把有PROCESS误当有PFS/全局依赖权限、把忙状态/1142改成0、把QS范围旧回执当AI资格均会误判。本入口独立防止这三类替代。元数据观察不跨查询原子；AI正在处理业务时允许非零，当前活动值不能用于直接批准DDL。

## 下一步建议

保持QS Production Deploy禁用、AI自动发布false。通过精确源码CI和原生fixture后仅合并工具源码，再在main派发固定只读operation，校验24查询、AI身份、原DDL/行数和全部释放；回执进入infra424。生产仍43，业务不停止、DDL不执行；正式窗口管理员观察必须重新取得，不能复用本次活库时点值。
