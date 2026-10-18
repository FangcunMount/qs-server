CREATE TABLE event_delivery_transport_evidence (
  identity_hash BINARY(32) NOT NULL PRIMARY KEY,
  provider VARCHAR(32) NOT NULL,
  topic_name VARCHAR(255) NOT NULL,
  channel_name VARCHAR(255) NOT NULL,
  transport_message_id VARCHAR(64) NOT NULL,
  application_id VARCHAR(255) NOT NULL,
  physical_topic VARCHAR(128) NOT NULL,
  original_wire LONGBLOB NOT NULL,
  metadata_bytes BLOB NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
