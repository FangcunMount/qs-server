# Task 待填写列表与入口发布说明

本说明覆盖小程序家庭档案下的待填写 Task，以及仅使用 task_id 的直接入口和微信开放提醒。代码位于 qs-server 与 qs-collection-system 的 `codex/task-inbox-id-entry` 分支；本轮没有执行生产部署、小程序上传或正式发布。微信真实 Task 提交、关联及提醒点击验收仍待新服务端环境。

## 改动与权威链路

首页按当前家庭档案查询 `GET /api/v1/plan-tasks?testee_id=...`。collection 从登录凭证取得 UserID，在查询前后校验有效 IAM ProfileLink；内部 PlanEntry 读取原 assessment_task 和 enrollment。没有新增任务账本。pending、opened 可展示；终态、已关联 assessment、超过硬失效时间的任务不展示，错过开放窗口的 pending 任务也不展示。

列表展示测评名称、所属计划、开放时间和填写截止时间。Plan 没有独立名称字段，所属计划用测评名称及计划编号展示。opened 的填写截止取 expire_at，pending 还未产生 expire_at 时显示 due_at；due_at 是计划履约时间，expire_at 才是现有入口硬失效时间，两者不合并改写。切换家庭档案、返回首页、下拉及前台每 15 秒刷新；失效后即使列表响应尚未刷新也不能点击开始。

Task 入口为 `pages/assessment/fill/index?task_id=...`，调用 `GET /api/v1/plan-task-entries/{task_id}`。服务端读取 Task 的机构、档案、量表，校验 opened、open_at、expire_at、assessment 关联及有效 enrollment；collection 再校验当前用户的档案关系。URL 上补传 token、问卷 code、模型 code、档案 ID 不改变这些事实，TaskID 不构成授权。

入口解析取 Task 量表的已发布模型绑定，返回精确 questionnaire_code、questionnaire_version 和 model_version；小程序按精确题版加载，并在开始作答时携带精确模型版本。版本或返回题版不匹配则拒绝，不能回退到外传 code 或默认最新题版。

开始作答仍用原 answering-start 准入，来源为 `plan_task`。提交保留 answering_start_id、origin_ref 和 task_id；Survey 原来源解析校验机构、档案和模型，本次补上开放时间、硬失效及已关联 assessment 检查。原 AnswerSheet 冻结 admission、来源快照、可靠受理和 Journey assessment intake 不改。intake 使用原 Task resolver / CompleteTask 建立关联；原完成路径是 best effort，须实际核对 Task completed 与 assessment_id，不能仅凭提交返回成功认定关联完成。

Task/Plan 当前没有独立的创建时模型版本快照；已有版本冻结点在作答开始及答卷准入。本改动保留这些冻结点，未新增 Task 创建时冻结机制，也不重写已受理答卷或历史事件的快照。

## 旧入口与历史提醒兼容

- 旧 Task 页面链接含 `task_id` 和 token：新版小程序优先按 TaskID 解析并丢弃入口 token；仍要求登录及有效档案关系。
- 旧 REST `/plan-task-entries/{task_id}/{token}` 保留；历史 token 参数忽略，不授权。内部 protobuf Token 字段保留并标为 deprecated，兼容旧 collection。
- 新 Task entry_token 为空，entry_url 的查询参数只有 task_id；新开放事件继承此无 token URL。
- 新构造的微信页面统一为 `pages/assessment/fill/index?task_id=...`。旧持久化 entry_url 即使含 token，也只提取 TaskID 用于新页面构造。
- 历史事件、Task 字段和已发送提醒不批量改写、不回填、更不重发。提醒 recipient snapshot、delivery ledger、调用标记、unknown 人工复核和发送截止策略保持原样；结果未知不能自动重发。
- 正常登录 access/refresh token、Authorization header，以及医生推荐 ae token 和其他扫码契约均保留。

旧小程序版本仍要求 Task token，不能消费新生成的无 token 链接。正式切换必须协调服务端与小程序发布时间，在旧客户端仍承担入口流量时不要启动新的无 token Task 提醒；不得通过重发历史通知弥补版本切换。该风险须在生产部署方案中明确，不能把“服务端先部署”理解为任意长时间的混合版本兼容。

## 文件责任与另一会话隔离

本会话只在两个独立 worktree 实施，不覆盖主工作区或其他 M5 M6 worktree。

交叉位置为 `task_opened_service.go` 的微信页面参数构造、`task_lifecycle.go` 的空入口 token 约束、Plan 模块入口 resolver 组合，以及 `answersheetattribution/resolver.go` 的用户来源时间准入。执行、恢复、提醒 ledger / batch / policy、worker handler、迁移及验收脚本保持原文件责任，未修改。gRPC 契约和 collection ACL 的改动仅服务新增档案任务查询；上线前对最新主线核对交叉 diff，不能覆盖另一会话的后续提交。

## 验证状态

- 服务端：Plan、Task lifecycle、notification、collection 全包、内部 gRPC、身份 ACL、survey 作答与提交、attribution、容器组合测试通过；Journey assessment intake 测试通过。两个服务构建及相关 go vet 通过。
- 新边界测试：仅 TaskID、旧 token 忽略、未开放与未来开放、完成／取消／过期、有效 enrollment 范围、越权档案、补传 code／档案、硬失效边界、无 token URL 及开放事件。
- 小程序：完整 verify:frontend 通过，覆盖类型、UI、接口契约、边界、资源、生产构建及包大小；新增 Task 参数优先、精确题版、作答来源、登录回跳、家庭成员旧响应、退出会话和终态刷新测试。最终 UI 为 79 个 suite、423 个测试通过。构建存在样式抽取顺序警告，实际界面与其他入口仍需微信工具复核。
- 数据库／部署：本轮没有跑真实 MySQL／Mongo 的提交联调，没有服务端镜像 CI／生产部署记录；数据库集成测试的可选跳过不计入真实验证。
- 微信工具：只读确认当前工具打开主工作区旧 dist 并连接生产。没有覆盖其构建、切换项目或提交真实 Task；提醒实际送达及点击也未验证。

## 发布与业务验收步骤

1. 先确定已授权的测试环境、新服务端版本及可消耗的真实测试 Task。服务端需同步 apiserver、collection 及 collection 证书允许的 ListParticipantTasks ACL；没有新增数据库迁移。生产部署须在本会话另获明确授权，并按现有部署 Runbook 准备 exact SHA、镜像 digest、回滚版本和兼容窗口。
2. 微信开发者工具单独导入本功能 worktree 的 dist，保留另一会话当前项目。确认接口指向测试环境；正式小程序上传与发布由用户完成。
3. 登录测试用户，在首页选择其有效家庭档案，从待填写列表进入 opened 测试 Task。核对名称、计划编号、开放／截止；网络请求只有 TaskID 定位，正常 Authorization 登录凭证继续存在。
4. 完成作答并提交，记录 task_id、answering_start_id、answer_sheet_id 和 assessment_id。等异步处理完成后从服务端核对 assessment 的 plan／task 来源及原模型版本；查询 assessment_task 的 status、assessment_id、completed_at，要求 completed 且关联相同 assessment。返回首页，列表应移除此任务。
5. 用自定义编译路径 `pages/assessment/fill/index`、参数 `task_id=<测试Task>` 验证直接进入；清理本测试项目登录态后再进入，登录／必要注册后应继续原 Task。不要清理其他项目的登录态。
6. 使用另一有效 Task 验证新开放提醒页面只有 TaskID，实际微信收到后点击并完成相同链路。提醒测试须使用新 Task／新开放事件，不重放既有 sent 或 unknown delivery。
7. 用无关系的测试用户、pending、未来 open_at、completed、canceled、expired 及到 expire_at 的 Task 测试入口、开始和提交拒绝；补传 q、mc、t、testee_id 不能绕过。有效用户保留原机构／enrollment 范围。
8. 验证旧含 token Task 链接在新版小程序可用；抽测医生推荐 ae token、普通问卷、人格和行为测评。分别保存代码 SHA、自动检查、已部署版本与实际业务结果；尚未执行的项目保持未验收。

真实业务关联可只读核对：

```sql
SELECT id, org_id, testee_id, plan_id, enrollment_id, status,
       assessment_id, open_at, expire_at, completed_at
FROM assessment_task
WHERE id = <测试TaskID> AND deleted_at IS NULL;
```

在记录中保留必要 ID 与版本，隐藏登录凭证、openid、个人资料及答案内容。本说明不授权生产部署或历史通知重发。
