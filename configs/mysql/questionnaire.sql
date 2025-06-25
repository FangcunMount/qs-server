--
-- Current Database: `questionnaire`
--

DROP DATABASE IF EXISTS `questionnaire`;

CREATE DATABASE IF NOT EXISTS `questionnaire` DEFAULT CHARACTER SET utf8;

USE `questionnaire`;

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