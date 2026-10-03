-- The host opens runtime admission explicitly after cutover prerequisites pass.
-- Shared submission locks and exclusive operator locks are held until commit.
CREATE TABLE ai_messaging_admission (
 singleton TINYINT UNSIGNED PRIMARY KEY,
 closed BOOLEAN NOT NULL,
 revision BIGINT UNSIGNED NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 CONSTRAINT ai_messaging_admission_singleton CHECK (singleton=1)
) ENGINE=InnoDB;
INSERT INTO ai_messaging_admission(singleton,closed,revision,updated_at)
VALUES(1,TRUE,0,UTC_TIMESTAMP(6));
