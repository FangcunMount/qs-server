// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Publish 发布一个 redis 事件到指定的 redis 通道，当某些操作发生时
func Publish() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Status() != http.StatusOK {
			log.L(c).Debugf("request failed with http status code `%d`, ignore publish message", c.Writer.Status())

			return
		}

		var resource string

		pathSplit := strings.Split(c.Request.URL.Path, "/")
		if len(pathSplit) > 2 {
			resource = pathSplit[2]
		}

		method := c.Request.Method
		log.L(c).Debugw("publish", "method", method, "resource", resource)

		switch resource {
		case "policies":
			// notify(c, method, load.NoticePolicyChanged)
		case "secrets":
			// notify(c, method, load.NoticeSecretChanged)
		default:
		}
	}
}

// notify 通知 redis 发布事件
// func notify(ctx context.Context, method string, command load.NotificationCommand) {
// 	switch method {
// 	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
// 		redisStore := &storage.RedisCluster{}
// 		message, _ := json.Marshal(load.Notification{Command: command})

// 		if err := redisStore.Publish(load.RedisPubSubChannel, string(message)); err != nil {
// 			log.L(ctx).Errorw("publish redis message failed", "error", err.Error())
// 		}
// 		log.L(ctx).Debugw("publish redis message", "method", method, "command", command)
// 	default:
// 	}
// }
