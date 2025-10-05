-- 用户主表
CREATE TABLE IF NOT EXISTS users (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  username        VARCHAR(64)  NULL,
  password        VARCHAR(128) NULL,
  nickname        VARCHAR(64)  NULL,
  avatar          VARCHAR(255) NULL,
  phone           VARCHAR(32)  NULL,
  email           VARCHAR(128) NULL,
  introduction    VARCHAR(500) NULL,
  status          TINYINT      NOT NULL DEFAULT 1 COMMENT '1正常/0禁用',
  created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_phone (phone),
  UNIQUE KEY uk_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户主表';

-- 微信应用表
CREATE TABLE IF NOT EXISTS wx_apps (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  name             VARCHAR(64)  NOT NULL COMMENT '自定义名称',
  platform         ENUM('mini','oa') NOT NULL COMMENT 'mini=小程序, oa=公众号',
  appid            VARCHAR(64)  NOT NULL COMMENT '微信AppID',
  secret           VARCHAR(128) NULL COMMENT 'AppSecret',
  token            VARCHAR(64)  NULL COMMENT '服务器配置Token',
  encoding_aes_key VARCHAR(64)  NULL COMMENT '消息加解密Key',
  mchid            VARCHAR(32)  NULL COMMENT '商户号',
  serial_no        VARCHAR(64)  NULL COMMENT '商户证书序列号',
  pay_cert_id      BIGINT       NULL COMMENT '指向密钥表',
  is_enabled       TINYINT      NOT NULL DEFAULT 1 COMMENT '是否启用',
  env              ENUM('prod','test','dev') NOT NULL DEFAULT 'prod' COMMENT '环境',
  remark           VARCHAR(255) NULL COMMENT '备注',
  created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_platform_appid (platform, appid),
  KEY idx_enabled (is_enabled, env)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信应用配置表';

-- 微信账户表
CREATE TABLE IF NOT EXISTS wx_accounts (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id         BIGINT       NULL COMMENT '绑定到哪位users.id',
  app_id          BIGINT       NOT NULL COMMENT '所属应用ID',
  appid           VARCHAR(64)  NOT NULL COMMENT '微信AppID',
  platform        ENUM('mini','oa') NOT NULL COMMENT 'mini=小程序, oa=公众号',
  openid          VARCHAR(64)  NOT NULL COMMENT '微信OpenID',
  unionid         VARCHAR(64)  NULL COMMENT '微信UnionID',
  nickname        VARCHAR(64)  NULL COMMENT '微信昵称',
  avatar_url      VARCHAR(255) NULL COMMENT '微信头像',
  session_key     VARCHAR(128) NULL COMMENT '小程序SessionKey',
  followed        TINYINT      NOT NULL DEFAULT 0 COMMENT '是否关注(仅OA)',
  followed_at     DATETIME     NULL COMMENT '关注时间(仅OA)',
  unfollowed_at   DATETIME     NULL COMMENT '取关时间(仅OA)',
  last_login_at   DATETIME     NULL COMMENT '最近登录时间',
  created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_app_platform_openid (appid, platform, openid),
  KEY idx_unionid (unionid),
  KEY idx_user (user_id),
  KEY idx_app (app_id),
  CONSTRAINT fk_wxacc_app FOREIGN KEY (app_id) REFERENCES wx_apps(id),
  CONSTRAINT fk_wxacc_user FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信账户表';

-- 账号合并日志
CREATE TABLE IF NOT EXISTS account_merge_logs (
  id        BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id   BIGINT       NOT NULL COMMENT '用户ID',
  wxacc_id  BIGINT       NOT NULL COMMENT '微信账户ID',
  reason    VARCHAR(64)  NOT NULL COMMENT '合并原因: unionid|phone|manual',
  created_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_user (user_id),
  KEY idx_acc (wxacc_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账号合并日志';

-- 微信证书密钥表（支付用）
CREATE TABLE IF NOT EXISTS wx_cert_keys (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  typ          ENUM('apiclient_key','platform_cert') NOT NULL COMMENT '密钥类型',
  version_tag  VARCHAR(64)  NOT NULL COMMENT '版本/序列号',
  pem_cipher   VARBINARY(8192) NOT NULL COMMENT '加密后的PEM',
  not_before   DATETIME NULL COMMENT '生效时间',
  not_after    DATETIME NULL COMMENT '失效时间',
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_type_ver (typ, version_tag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信支付证书密钥表';
