# 答卷 GRPC API 文档

## 概述

答卷服务提供了完整的答卷管理功能，包括保存、查询和更新答卷信息。

## 接口列表

### 1. SaveAnswerSheet
保存新的答卷。

### 2. GetAnswerSheet
根据ID获取答卷详情。

### 3. ListAnswerSheets
获取答卷列表。

### 4. SaveAnswerSheetScores ⭐ 新增
保存答卷答案和分数。

## SaveAnswerSheetScores 接口详情

### 接口描述
用于保存已计算完成的答卷答案和分数。这个接口通常由evaluation-server调用，在完成答卷分数计算后更新数据库中的答卷信息。

### 请求参数
```protobuf
message SaveAnswerSheetScoresRequest {
  uint64 answer_sheet_id = 1;  // 答卷ID
  uint32 total_score = 2;      // 总分
  repeated Answer answers = 3;  // 答案列表（包含分数）
}
```

### 响应参数
```protobuf
message SaveAnswerSheetScoresResponse {
  uint64 answer_sheet_id = 1;  // 答卷ID
  uint32 total_score = 2;      // 总分
  string message = 3;          // 响应消息
}
```

### 使用场景
1. **自动评分**：evaluation-server接收到答卷保存消息后，自动计算分数并调用此接口保存结果
2. **手动评分**：管理员手动评分后调用此接口更新分数
3. **分数修正**：发现评分错误时，重新计算并调用此接口更新

### 调用示例
```go
// 创建客户端
client := NewAnswerSheetClient(factory)

// 准备数据
answerSheetID := uint64(12345)
totalScore := uint32(100)
answers := []*answersheet.Answer{
    {
        QuestionCode: "Q1",
        QuestionType: "Radio",
        Score:        5,
        Value:        "\"option1\"",
    },
    // ... 更多答案
}

// 调用接口
err := client.SaveAnswerSheetScores(ctx, answerSheetID, totalScore, answers)
if err != nil {
    log.Errorf("保存分数失败: %v", err)
    return err
}
```

### 错误处理
- `codes.Internal`: 服务器内部错误
- `codes.NotFound`: 答卷不存在
- `codes.InvalidArgument`: 参数无效

## 数据流程

1. **collection-server** 保存答卷原始数据
2. **evaluation-server** 接收消息，计算分数
3. **evaluation-server** 调用 `SaveAnswerSheetScores` 保存计算结果
4. **apiserver** 更新数据库中的答卷分数信息

## 注意事项

1. 此接口会覆盖答卷中现有的分数信息
2. 答案列表必须包含所有问题的答案和分数
3. 总分应该与所有答案分数之和一致
4. 调用前确保答卷ID存在且有效 