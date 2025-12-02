// Package main 演示任务队列模式（Task Queue Pattern）
// 多个 Worker 负载均衡处理任务
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

// EmailTask 邮件发送任务
type EmailTask struct {
	TaskID  string `json:"task_id"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func main() {
	log.Println("=== 任务队列模式演示 ===")
	log.Println("场景：10 个 Worker 负载均衡处理 100 个邮件发送任务")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	// ========== 关键点：所有 Worker 使用相同的 channel ==========
	// 这样每条消息只会被一个 Worker 接收（负载均衡）

	workerCount := 10
	taskCount := 100

	log.Printf("启动 %d 个 Worker...\n", workerCount)

	// 启动多个 Worker
	for i := 1; i <= workerCount; i++ {
		workerID := i
		go startWorker(bus, workerID)
	}

	// 等待 Worker 准备好
	time.Sleep(2 * time.Second)

	log.Printf("\n开始生产 %d 个任务...\n", taskCount)

	// ========== 生产任务 ==========
	publisher := bus.Publisher()
	startTime := time.Now()

	for i := 1; i <= taskCount; i++ {
		task := EmailTask{
			TaskID:  fmt.Sprintf("TASK-%04d", i),
			To:      fmt.Sprintf("user%d@example.com", i),
			Subject: "Welcome!",
			Body:    fmt.Sprintf("Hello User %d, welcome to our service!", i),
		}

		payload, _ := json.Marshal(task)
		msg := messaging.NewMessage("", payload)
		msg.Metadata["task_type"] = "email.send"
		msg.Metadata["priority"] = "normal"

		publisher.PublishMessage(context.Background(), "task.email", msg)

		// 每 10 个任务打印一次进度
		if i%10 == 0 {
			log.Printf("  ⏳ 已创建 %d/%d 个任务", i, taskCount)
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("\n✅ 所有任务创建完成！")
	log.Printf("   耗时: %v", elapsed)
	log.Printf("   速度: %.0f 任务/秒\n", float64(taskCount)/elapsed.Seconds())

	log.Println("等待所有任务处理完成...")
	log.Println("（观察任务如何在 Worker 之间负载均衡）")

	// 等待一段时间让任务处理完成
	time.Sleep(15 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")

	// 等待退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// startWorker 启动一个 Worker
func startWorker(bus messaging.EventBus, workerID int) {
	var processedCount int64

	// ========== 关键点：使用相同的 channel "email-workers" ==========
	err := bus.Subscriber().Subscribe("task.email", "email-workers",
		func(ctx context.Context, msg *messaging.Message) error {
			var task EmailTask
			json.Unmarshal(msg.Payload, &task)

			// 模拟邮件发送（耗时操作）
			time.Sleep(100 * time.Millisecond)

			// 原子递增计数器
			count := atomic.AddInt64(&processedCount, 1)

			// 每处理 5 条消息打印一次
			if count%5 == 0 {
				log.Printf("  🔧 [Worker-%02d] 已处理 %d 条任务 (最新: %s → %s)",
					workerID, count, task.TaskID, task.To)
			}

			return msg.Ack()
		})

	if err != nil {
		log.Printf("Worker-%d 启动失败: %v", workerID, err)
		return
	}

	log.Printf("  ✓ Worker-%02d 已启动", workerID)
}

// 预期输出：
//
// === 任务队列模式演示 ===
// 场景：10 个 Worker 负载均衡处理 100 个邮件发送任务
//
// 启动 10 个 Worker...
//   ✓ Worker-01 已启动
//   ✓ Worker-02 已启动
//   ✓ Worker-03 已启动
//   ✓ Worker-04 已启动
//   ✓ Worker-05 已启动
//   ✓ Worker-06 已启动
//   ✓ Worker-07 已启动
//   ✓ Worker-08 已启动
//   ✓ Worker-09 已启动
//   ✓ Worker-10 已启动
//
// 开始生产 100 个任务...
//   ⏳ 已创建 10/100 个任务
//   ⏳ 已创建 20/100 个任务
//   ...
//   ⏳ 已创建 100/100 个任务
//
// ✅ 所有任务创建完成！
//    耗时: 123ms
//    速度: 813 任务/秒
//
// 等待所有任务处理完成...
// （观察任务如何在 Worker 之间负载均衡）
//
//   🔧 [Worker-03] 已处理 5 条任务 (最新: TASK-0023 → user23@example.com)
//   🔧 [Worker-07] 已处理 5 条任务 (最新: TASK-0041 → user41@example.com)
//   🔧 [Worker-01] 已处理 5 条任务 (最新: TASK-0018 → user18@example.com)
//   🔧 [Worker-05] 已处理 10 条任务 (最新: TASK-0056 → user56@example.com)
//   ...
//
// 核心知识点：
//
// 1. 任务队列特点
//    - 多个 Worker 竞争消费任务
//    - 每个任务只会被一个 Worker 处理
//    - 自动负载均衡
//    - 提高并发处理能力
//
// 2. Channel 的作用
//    - 所有 Worker 使用相同的 channel（email-workers）
//    - 消息中间件自动分配任务给不同的 Worker
//    - 类似于 RabbitMQ 的 Queue 或 Kafka 的 Consumer Group
//
// 3. 适用场景
//    ✅ 邮件发送（大量邮件需要并发发送）
//    ✅ 图片处理（缩略图生成、水印添加）
//    ✅ 数据导出（大文件生成、报表导出）
//    ✅ 视频转码（视频格式转换、压缩）
//    ✅ 爬虫任务（URL 抓取、数据解析）
//
// 4. 与事件驱动的区别
//    任务队列：多个 Worker，只有一个收到消息（负载均衡）
//    事件驱动：多个服务，每个都收到消息（广播）
//
// 5. 性能优化
//    - 根据 CPU 核心数调整 Worker 数量
//    - 调整 MaxInFlight 控制并发数
//    - 使用批量发布提高吞吐量
//    - 监控任务堆积情况，动态扩容
//
// 最佳实践：
// ✅ Worker 数量 = CPU 核心数 × 2（经验值）
// ✅ 任务要具有幂等性（可重试）
// ✅ 记录处理失败的任务（死信队列）
// ✅ 监控任务处理时间和成功率
// ✅ 使用优先级队列处理紧急任务
