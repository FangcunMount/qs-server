package main

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// ==================== 受试者相关类型定义 ====================

// ==================== 受试者 Seed 函数 ====================

// seedTesteeCenter 创建受试者数据
//
// 业务说明：
// 1. 从配置文件读取受试者数据
// 2. 创建或更新受试者记录
// 3. 将受试者 ID 存储到上下文中，供后续步骤使用
//
// 幂等性：通过查询检查，已存在的受试者会被更新而不是重复创建
func seedTesteeCenter(ctx context.Context, deps *dependencies, state *seedContext) error {
	logger := deps.Logger
	config := deps.Config

	if len(config.Testees) == 0 {
		logger.Infow("No testees to seed")
		return nil
	}

	logger.Infow("Seeding testees", "count", len(config.Testees))

	for i, tc := range config.Testees {
		logger.Debugw("Processing testee", "index", i+1, "name", tc.Name)

		// 解析生日
		var birthday *time.Time
		if tc.Birthday != "" {
			bd, err := ParseDate(tc.Birthday)
			if err != nil {
				logger.Warnw("Invalid birthday format, skipping", "testee", tc.Name, "birthday", tc.Birthday, "error", err)
			} else {
				birthday = &bd
			}
		}

		// 解析性别
		gender := ParseGender(tc.Gender)

		// 构建 PO
		po := &actor.TesteePO{
			OrgID:            config.Global.OrgID,
			ProfileID:        tc.ProfileID,
			Name:             tc.Name,
			Gender:           gender,
			Birthday:         birthday,
			Tags:             tc.Tags,
			Source:           tc.Source,
			IsKeyFocus:       tc.IsKeyFocus,
			TotalAssessments: 0,
		}

		// 设置默认值
		if po.Source == "" {
			po.Source = "seed_data"
		}
		if po.Tags == nil {
			po.Tags = make([]string, 0)
		}

		// 检查是否已存在（通过 name + orgID）
		var existing actor.TesteePO
		result := deps.MySQLDB.Where("name = ? AND org_id = ?", tc.Name, config.Global.OrgID).First(&existing)

		if result.Error == nil {
			// 已存在，更新
			logger.Debugw("Testee already exists, updating", "testee", tc.Name, "id", existing.ID)

			po.ID = existing.ID
			po.Version = existing.Version
			po.CreatedAt = existing.CreatedAt
			po.CreatedBy = existing.CreatedBy

			if err := deps.MySQLDB.Save(po).Error; err != nil {
				return fmt.Errorf("failed to update testee %s: %w", tc.Name, err)
			}

			state.TesteeIDsByName[tc.Name] = fmt.Sprintf("%d", po.ID)
			logger.Infow("Testee updated", "name", tc.Name, "id", po.ID)
		} else {
			// 不存在，创建
			// ID 由 GORM 自动生成
			po.Version = mysql.InitialVersion

			if err := deps.MySQLDB.Create(po).Error; err != nil {
				return fmt.Errorf("failed to create testee %s: %w", tc.Name, err)
			}

			state.TesteeIDsByName[tc.Name] = fmt.Sprintf("%d", po.ID)
			logger.Infow("Testee created", "name", tc.Name, "id", po.ID)
		}
	}

	logger.Infow("Testees seeded successfully", "count", len(config.Testees))
	return nil
}

// ==================== 辅助函数 ====================
