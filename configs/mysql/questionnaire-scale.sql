--
-- Current Database: `miniblog`
--

DROP DATABASE IF EXISTS `miniblog`;

CREATE DATABASE IF NOT EXISTS `miniblog` DEFAULT CHARACTER SET utf8;

USE `miniblog`;

--
-- Table structure for table `user`
--

DROP TABLE IF EXISTS `user`;
CREATE TABLE `user` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(255) NOT NULL,
  `password` varchar(255) NOT NULL,
  `nickname` varchar(30) NOT NULL,
  `avatar` varchar(255) NOT NULL,
  `email` varchar(256) NOT NULL,
  `phone` varchar(16) NOT NULL,
  `introduction` varchar(1024) NOT NULL,
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '1: 正常, 2: 禁用',
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  `updated_at` timestamp NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (`id`),
  UNIQUE KEY `username` (`username`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8;

--
-- Table structure for table `module`
--
DROP TABLE IF EXISTS `module`;
CREATE TABLE `module` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(255) NOT NULL,
  `title` varchar(255) NOT NULL,
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '1: 上架, 2: 下架',
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  `updated_at` timestamp NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (`id`),
  UNIQUE KEY `code` (`code`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8;

--
-- Table structure for table `section`
--
DROP TABLE IF EXISTS `section`;
CREATE TABLE `section` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(255) NOT NULL,
  `title` varchar(255) NOT NULL,
  `sort` int(11) NOT NULL DEFAULT '0',
  `module_code` varchar(255) NOT NULL,
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '1: 上架, 2: 下架',
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  `updated_at` timestamp NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (`id`),
  UNIQUE KEY `module_code_code` (`module_code`, `code`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8;

--
-- Table structure for table `article`
--
DROP TABLE IF EXISTS `article`;
CREATE TABLE `article` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `title` varchar(255) NOT NULL,
  `content` longtext NOT NULL,
  `section_code` varchar(255) NOT NULL,
  `author` varchar(255) NOT NULL,
  `tags` varchar(255) NOT NULL,
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '1: 正常, 2: 禁用',
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  `updated_at` timestamp NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (`id`),
  UNIQUE KEY `section_code_id` (`section_code`, `id`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8;

--
-- Insert data into table `user`
--
INSERT INTO `user` (`username`, `password`, `nickname`, `email`, `phone`, `status`) VALUES ('clack', '1q2w3e4r', '杨舒杰', 'yshujie@163.com', '15711236163', 1);


--
-- Insert data into table `module`
--
INSERT INTO `module` (`code`, `title`, `status`) VALUES ('ai', 'AI', 0);
INSERT INTO `module` (`code`, `title`, `status`) VALUES ('go', 'Golang', 1);
INSERT INTO `module` (`code`, `title`, `status`) VALUES ('ddd', '领域驱动设计', 1);
INSERT INTO `module` (`code`, `title`, `status`) VALUES ('refactor', '重构', 0);
INSERT INTO `module` (`code`, `title`, `status`) VALUES ('database', '数据库', 1);

--
-- Insert data into table `section`
--
# AI
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('ai_history', '人工智能发展史', 'ai', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('ai_prompt', 'Prompt 工程', 'ai', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('ai_llm', 'LLM 模型', 'ai', 1);

# Golang
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('go_base', 'Golang 基础', 'go', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('go_concurrent', 'Golang 中的高并发设计', 'go', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('go_design_pattern', 'Golang 中的设计模式', 'go', 1);

# 领域驱动设计
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('analysis', '需求分析', 'ddd', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('modeling', '领域建模', 'ddd', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('architecture_base', '架构设计', 'ddd', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('design_pattern', '设计模式', 'ddd', 1);


# 软件架构设计
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('architecture_base', '架构基础', 'architecture', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('design_thinking', '设计原则&思想', 'architecture', 1);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('design_pattern', '设计模式', 'architecture', 1);

# 重构
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('refactor_smell', '坏味道', 'refactor', 0);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('refactor_technique', '重构技巧', 'refactor', 0);

# 数据库
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('mysql', 'MySQL', 'refactor', 0);
INSERT INTO `section` (`code`, `title`, `module_code`, `status`) VALUES ('redis', 'Redis', 'refactor', 0);