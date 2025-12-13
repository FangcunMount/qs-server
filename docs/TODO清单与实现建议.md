# TODO 清单与实现建议

> 生成时间：2025-01-XX  
> 代码版本：当前开发版本

本文档梳理了代码中所有 `TODO` 标记，并按优先级和功能域分类，为后续开发提供指导。

---

## 1. 缓存优化（已完成）

### ✅ 量表缓存预热

- **位置**：`internal/worker/handlers/scale_handler.go`
- **状态**：已添加策略说明
- **决策**：采用 Lazy Loading + Cache-Aside 模式，无需预热缓存
- **理由**：
  - 当前架构下，Worker 无 MongoDB 访问权限
  - 缓存读写由 apiserver repository 层负责
  - 预热缓存需要跨服务协调，成本较高
  - 懒加载对低频访问量表更经济

### ✅ 问卷缓存预热

- **位置**：`internal/worker/handlers/questionnaire_handler.go`
- **状态**：已添加策略说明
- **决策**：同量表缓存策略
- **特殊考虑**：问卷数据结构大，预热会占用较多 Redis 空间

---

## 2. 高优先级（核心功能）

### P0 - 权限验证

#### 答卷提交权限校验

- **位置**：`internal/apiserver/interface/grpc/service/answersheet.go:89`
- **当前问题**：缺少用户身份验证和权限校验
- **建议实现**：

  ```go
  // 1. 从 gRPC Context 提取 JWT Token
  // 2. 调用 IAM 验证 Token 有效性
  // 3. 检查用户是否有填写该问卷的权限
  // 4. 对于 proxy 填写场景，验证代填者和受试者关系
  ```

- **依赖**：IAM 服务集成

---

### P1 - 报告解读服务

#### 调用 InterpretService 获取结论

- **位置**：
  - `internal/apiserver/domain/evaluation/report/builder.go:84`
  - `internal/apiserver/domain/evaluation/report/builder.go:112`
  - `internal/apiserver/domain/evaluation/report/builder.go:130`
- **当前问题**：报告中的解读文本为硬编码或空值
- **建议实现**：

  ```go
  // 选项 A：内部知识库
  type InterpretService interface {
      GetInterpretation(scaleCode, factorCode string, score float64) (string, error)
      GetSuggestion(scaleCode, factorCode string, level string) ([]string, error)
  }

  // 选项 B：外部 AI 服务
  // 调用 GPT/Claude API，传入量表定义和分数，生成解读
  ```

- **数据来源**：
  - 量表文档的标准解读（存储在 MongoDB 或配置文件）
  - 心理咨询专家编写的解读模板
  - AI 辅助生成（需人工审核）

#### 从知识库获取建议

- **位置**：`internal/apiserver/domain/evaluation/report/suggestion.go:117`
- **实现方式**：

  ```yaml
  # configs/suggestions.yaml
  scales:
    SCL-90:
      factors:
        depression:
          high:
            - 建议咨询专业心理医生
            - 保持规律作息，避免熬夜
          moderate:
            - 多参加户外活动
            - 与亲友保持沟通
  ```

---

### P1 - 报告导出

#### PDF 导出功能

- **位置**：`internal/apiserver/application/evaluation/assessment/report_query_service.go:69`
- **当前问题**：仅支持 JSON 返回，无 PDF/图片导出
- **技术选型**：
  1. **Go HTML 模板 + wkhtmltopdf**
     - 优点：简单，可复用现有模板
     - 缺点：依赖外部二进制，样式控制复杂
  2. **gofpdf / gopdf**
     - 优点：纯 Go 实现，易部署
     - 缺点：需要手动绘制每个元素，维护成本高
  3. **Headless Chrome (chromedp)**
     - 优点：渲染效果好，支持复杂样式
     - 缺点：资源占用高，需容器化部署
  4. **第三方服务（如云打印 API）**
     - 优点：高质量输出，无需维护
     - 缺点：增加外部依赖，可能有成本

**推荐方案**：HTML 模板 + Headless Chrome（chromedp）

```go
// 实现示例
type ReportExporter interface {
    ExportToPDF(report *Report) ([]byte, error)
    ExportToImage(report *Report) ([]byte, error)
}

// 使用 chromedp
func (e *ChromeExporter) ExportToPDF(report *Report) ([]byte, error) {
    html := renderTemplate(report)
    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()
    
    var pdfData []byte
    if err := chromedp.Run(ctx, chromedp.Tasks{
        chromedp.Navigate("data:text/html," + url.QueryEscape(html)),
        chromedp.WaitReady("body"),
        chromedp.ActionFunc(func(ctx context.Context) error {
            var err error
            pdfData, _, err = page.PrintToPDF().Do(ctx)
            return err
        }),
    }); err != nil {
        return nil, err
    }
    return pdfData, nil
}
```

#### ReportExporter 实现

- **位置**：`internal/apiserver/container/assembler/evaluation.go:134`
- **说明**：当前使用 stub 实现，需替换为上述真实实现

---

## 3. 中优先级（业务完善）

### P2 - 通知系统

#### 预警通知

- **位置**：
  - `internal/worker/handlers/assessment_handler.go:157`
  - `internal/worker/handlers/report_handler.go:73`
- **触发条件**：
  - 高风险因子检测（如抑郁、焦虑超过阈值）
  - 自杀风险评估结果异常
- **通知渠道**：
  - 站内消息（存储到数据库）
  - 短信通知（关键预警）
  - 邮件通知
  - 企业微信/钉钉机器人
- **实现建议**：

  ```go
  type NotificationService interface {
      SendAlert(testeeID uint64, alertType string, content string) error
      SendReport(testeeID uint64, reportID uint64) error
  }

  // 集成第三方服务
  // - 短信：阿里云 SMS / 腾讯云 SMS
  // - 邮件：SendGrid / AWS SES
  // - 企业通讯：企业微信 Webhook
  ```

#### 报告生成通知

- **位置**：`internal/worker/handlers/report_handler.go:76`
- **场景**：
  - 测评完成后自动生成报告
  - 通知受试者查看报告
  - 通知心理咨询师有新报告待审核

#### 监控告警

- **位置**：`internal/worker/handlers/assessment_handler.go:181`
- **场景**：
  - 评估失败超过阈值（如 10分钟内失败5次）
  - 消息队列堆积告警
  - 系统异常（如 gRPC 超时）
- **集成**：Prometheus Alertmanager / PagerDuty

---

### P2 - 统计功能

#### 平均分计算

- **位置**：`internal/apiserver/application/evaluation/assessment/management_service.go:133`
- **需求**：在测评列表中显示平均分，用于快速筛选
- **实现**：

  ```go
  func (s *ManagementService) calculateAverageScore(assessmentID uint64) (*float64, error) {
      scores, err := s.assessmentRepo.GetAllScores(ctx, assessmentID)
      if err != nil || len(scores) == 0 {
          return nil, err
      }
      
      sum := 0.0
      for _, score := range scores {
          sum += score.Score
      }
      avg := sum / float64(len(scores))
      return &avg, nil
  }
  ```

#### 复杂条件查询

- **位置**：`internal/apiserver/application/evaluation/assessment/management_service.go:71`
- **需求**：
  - 按量表类型筛选
  - 按分数范围筛选（如 60-80分）
  - 按时间范围筛选
  - 按高风险因子筛选
- **实现**：构建动态 SQL 查询

#### 受试者测评统计

- **位置**：
  - `internal/apiserver/domain/actor/testee/counter.go:34`
  - `internal/apiserver/domain/actor/testee/counter.go:102`
- **统计项**：
  - 测评总次数
  - 最近测评时间
  - 风险等级分布
  - 问卷完成率
- **实现方式**：

  ```go
  // 定时任务：每天凌晨统计
  // 或实时统计：每次测评完成后更新
  type TesteeStatistics struct {
      TesteeID              uint64
      TotalAssessments      int
      CompletedCount        int
      HighRiskCount         int
      LatestAssessmentDate  time.Time
  }
  ```

---

### P2 - 领域事件

#### Staff 领域事件

- **位置**：
  - `internal/apiserver/domain/actor/staff/lifecycler.go:46`（创建事件）
  - `internal/apiserver/domain/actor/staff/lifecycler.go:69`（更新事件）
- **事件类型**：
  - `staff.created`
  - `staff.updated`
  - `staff.deleted`
- **用途**：
  - 同步到其他系统（如 HR 系统）
  - 审计日志记录
  - 权限变更通知

#### Testee 统计事件

- **位置**：`internal/apiserver/domain/actor/testee/counter.go:94`
- **事件类型**：
  - `testee.statistics_updated`
- **用途**：
  - 触发标签自动打标（`counter.go:88`）
  - 生成数据看板

---

## 4. 低优先级（辅助功能）

### P3 - 审计日志

- **位置**：`internal/worker/handlers/report_handler.go:98`
- **记录内容**：
  - 谁在何时查看了哪份报告
  - 谁修改了测评结果
  - 敏感操作记录
- **实现方式**：
  - 写入专用的审计日志表
  - 或集成第三方日志服务（如 AWS CloudTrail）

### P3 - 健康检查

- **位置**：`internal/apiserver/container/assembler/evaluation.go:225`
- **检查项**：
  - 数据库连接状态
  - Redis 连接状态
  - gRPC 服务可用性
  - 消息队列状态
- **实现**：

  ```go
  type HealthChecker interface {
      Check(ctx context.Context) HealthStatus
  }

  type HealthStatus struct {
      MongoDB   bool
      Redis     bool
      GRPC      bool
      NSQ       bool
      Timestamp time.Time
  }
  ```

### P3 - 答卷评分

- **位置**：`internal/apiserver/interface/grpc/service/answersheet.go:145`
- **说明**：当前评分在评估阶段进行，此处可能指人工评分或重新评分功能
- **实现场景**：
  - 开放题的人工评分
  - 评分规则调整后的重新评分

---

## 5. 架构改进（长期优化）

### 发号器替换

- **位置**：`internal/pkg/meta/id.go:26`
- **当前实现**：简单递增ID
- **建议方案**：
  - Snowflake 算法（分布式ID生成）
  - UUID v7（时间排序 + 随机）
  - 数据库自增（简单场景）

### ACL 规则配置化

- **位置**：`internal/pkg/grpc/server.go:184`
- **当前实现**：硬编码 ACL
- **改进方向**：

  ```yaml
  # configs/acl.yaml
  services:
    - name: "AnswerSheetService"
      methods:
        - name: "SaveAnswerSheet"
          allowed_roles: ["testee", "proxy"]
        - name: "SubmitAnswerSheet"
          allowed_roles: ["testee", "proxy", "admin"]
  ```

### MongoDB 驱动现代化

- **位置**：`internal/apiserver/database.go:321`
- **说明**：当前代码已使用 `go.mongodb.org/mongo-driver`，此 TODO 为历史遗留

### 用户服务集成

- **位置**：`internal/apiserver/domain/actor/staff/binder.go:102`
- **建议**：在绑定用户关系前，调用 IAM 服务验证用户存在性

### Testee 注册服务

- **位置**：`internal/apiserver/interface/restful/handler/actor.go:435`
- **说明**：`CreateTestee` API 已废弃，未来通过专门的 Registration Service 实现

### 多租户支持

- **位置**：`internal/collection-server/infra/iam/guardianship.go:141`
- **说明**：未来如需支持多租户，通过 IAM SDK 获取用户所属机构

---

## 6. 实施建议

### 阶段 1：核心功能补全（1-2周）

- ✅ 缓存优化策略（已完成）
- ⏳ 权限验证（答卷提交）
- ⏳ PDF 导出基础实现
- ⏳ 通知系统框架搭建

### 阶段 2：业务完善（2-3周）

- ⏳ 报告解读服务（知识库构建）
- ⏳ 统计功能实现
- ⏳ 领域事件发布
- ⏳ 预警通知集成

### 阶段 3：长期优化（持续迭代）

- ⏳ 审计日志
- ⏳ 健康检查
- ⏳ 架构改进

### 开发原则

1. **最小可用产品（MVP）**：优先实现核心功能，避免过度设计
2. **渐进式增强**：先实现基础功能，再逐步优化
3. **配置驱动**：尽量使用配置文件而非硬编码
4. **依赖解耦**：通过接口抽象外部依赖，便于测试和替换
5. **文档先行**：在实现前编写设计文档，明确需求和边界

---

## 7. 技术栈推荐

### 通知服务

- **短信**：阿里云 SMS / Twilio
- **邮件**：SendGrid / AWS SES / SMTP
- **企业通讯**：企业微信 Webhook / 钉钉机器人

### PDF 生成

- **推荐**：chromedp + HTML 模板
- **备选**：wkhtmltopdf / gofpdf

### 知识库

- **结构化存储**：MongoDB（JSONB 字段）
- **配置文件**：YAML（适合静态内容）
- **AI 辅助**：OpenAI API / Claude API（动态生成）

### 审计日志

- **轻量方案**：专用数据库表
- **企业方案**：AWS CloudTrail / ELK Stack

### 监控告警

- **指标采集**：Prometheus
- **告警管理**：Alertmanager
- **可视化**：Grafana

---

## 8. 附录：待决策问题

1. **报告解读内容来源**：自建知识库 vs AI 生成？
2. **PDF 渲染方案**：服务端渲染 vs 客户端生成？
3. **通知服务部署**：自建 vs SaaS？
4. **审计日志保留期限**：多久？存储成本考虑？
5. **多租户支持时间表**：是否在 MVP 阶段实现？

---

**文档维护者**：开发团队  
**更新频率**：每月或代码重大变更时
