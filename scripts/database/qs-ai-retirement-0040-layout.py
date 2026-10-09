"""Fixed 0040 physical contract and private lossless projection, without I/O.

Source: FangcunMount/qs-ai 82ffa1b43308f23fbb1ebe669c3071e0486e105a,
maintenance/schema_refactor/{v0040.json,layouts.py,conversion.py}.
The full physical scan and separately approved metadata remain authoritative;
logical aliases are never substituted for physical PKs, source hashes or bounds.
"""
import hashlib
import json
import re
import struct

HEAD = "0040_module_table_names"
SOURCE_SHA = "82ffa1b43308f23fbb1ebe669c3071e0486e105a"
CONTRACT_SHA256 = "1920802a64a4410fbd863873231d69694bc92b1ed36102191fc922975e837e92"
_SOURCE_CONTRACT = r"""{
  "head": "0040_module_table_names",
  "tables": [
    {
      "name": "evaluation_checkpoints",
      "create_sql": "\nCREATE TABLE evaluation_checkpoints (\n\trun_id CHAR(36) NOT NULL, \n\tversion BIGINT NOT NULL, \n\tcheckpoint_json JSON, \n\tCONSTRAINT pk_evaluation_checkpoints PRIMARY KEY (run_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "run_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "version",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "checkpoint_json",
          "type": "JSON",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "evaluation_completions",
      "create_sql": "\nCREATE TABLE evaluation_completions (\n\tkind VARCHAR(16) COLLATE ascii_bin NOT NULL, \n\trun_id CHAR(36) NOT NULL, \n\texecution_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tinvocation_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tcase_id VARCHAR(128) COLLATE utf8mb4_bin, \n\tslot_ordinal INTEGER, \n\texecution_ordinal INTEGER NOT NULL, \n\tcandidate_id VARCHAR(128) COLLATE utf8mb4_bin, \n\tcandidate_json JSON, \n\tevidence_json JSON NOT NULL, \n\tresult_json JSON, \n\traw_output MEDIUMBLOB NOT NULL, \n\tnormalized_output MEDIUMBLOB NOT NULL, \n\tgen_candidate_key VARCHAR(128) COLLATE utf8mb4_bin GENERATED ALWAYS AS (CASE WHEN kind='generation' THEN candidate_id ELSE NULL END) VIRTUAL, \n\tgen_case_key VARCHAR(128) COLLATE utf8mb4_bin GENERATED ALWAYS AS (CASE WHEN kind='generation' THEN case_id ELSE NULL END) VIRTUAL, \n\tgen_slot_key INTEGER GENERATED ALWAYS AS (CASE WHEN kind='generation' THEN slot_ordinal ELSE NULL END) VIRTUAL, \n\tsem_candidate_key VARCHAR(128) COLLATE utf8mb4_bin GENERATED ALWAYS AS (CASE WHEN kind='semantic' THEN candidate_id ELSE NULL END) VIRTUAL, \n\tCONSTRAINT pk_evaluation_completions PRIMARY KEY (kind, run_id, execution_id), \n\tCONSTRAINT ck_evaluation_completions_shape CHECK ((kind='generation' AND case_id IS NOT NULL AND slot_ordinal IS NOT NULL AND result_json IS NULL) OR (kind='semantic' AND candidate_id IS NOT NULL AND case_id IS NULL AND slot_ordinal IS NULL AND candidate_json IS NULL)), \n\tCONSTRAINT uk_evaluation_completions_run_id_gen_candidate_key UNIQUE (run_id, gen_candidate_key), \n\tCONSTRAINT uk_evaluation_completions_run_id_gen_case_key_gen_slot__2792b280 UNIQUE (run_id, gen_case_key, gen_slot_key, execution_ordinal), \n\tCONSTRAINT uk_evaluation_completions_run_id_sem_candidate_key_exec_e97c7f78 UNIQUE (run_id, sem_candidate_key, execution_ordinal), \n\tCONSTRAINT uk_evaluation_completions_kind_run_id_invocation_id UNIQUE (kind, run_id, invocation_id), \n\tCONSTRAINT ck_evaluation_completions_kind CHECK (kind IN ('generation','semantic'))\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_evaluation_completions_kind_run_id_candidate_id_exe_06f69b1e ON evaluation_completions (kind, run_id, candidate_id, execution_ordinal)",
        "CREATE INDEX idx_evaluation_completions_kind_run_id_case_id_slot_ord_16982959 ON evaluation_completions (kind, run_id, case_id, slot_ordinal, execution_ordinal)"
      ],
      "columns": [
        {
          "name": "kind",
          "type": "VARCHAR(16) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "execution_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "invocation_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "case_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "slot_ordinal",
          "type": "INTEGER",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "execution_ordinal",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "candidate_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "candidate_json",
          "type": "JSON",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "evidence_json",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "result_json",
          "type": "JSON",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "raw_output",
          "type": "MEDIUMBLOB",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "normalized_output",
          "type": "MEDIUMBLOB",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "gen_candidate_key",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": "CASE WHEN kind='generation' THEN candidate_id ELSE NULL END",
          "persisted": false
        },
        {
          "name": "gen_case_key",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": "CASE WHEN kind='generation' THEN case_id ELSE NULL END",
          "persisted": false
        },
        {
          "name": "gen_slot_key",
          "type": "INTEGER",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": "CASE WHEN kind='generation' THEN slot_ordinal ELSE NULL END",
          "persisted": false
        },
        {
          "name": "sem_candidate_key",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": "CASE WHEN kind='semantic' THEN candidate_id ELSE NULL END",
          "persisted": false
        }
      ],
      "primary_key": [
        "kind",
        "run_id",
        "execution_id"
      ],
      "indexes": [
        {
          "name": "idx_evaluation_completions_kind_run_id_case_id_slot_ord_16982959",
          "columns": [
            "kind",
            "run_id",
            "case_id",
            "slot_ordinal",
            "execution_ordinal"
          ],
          "unique": false
        },
        {
          "name": "idx_evaluation_completions_kind_run_id_candidate_id_exe_06f69b1e",
          "columns": [
            "kind",
            "run_id",
            "candidate_id",
            "execution_ordinal"
          ],
          "unique": false
        },
        {
          "name": "uk_evaluation_completions_run_id_sem_candidate_key_exec_e97c7f78",
          "columns": [
            "run_id",
            "sem_candidate_key",
            "execution_ordinal"
          ],
          "unique": true
        },
        {
          "name": "uk_evaluation_completions_kind_run_id_invocation_id",
          "columns": [
            "kind",
            "run_id",
            "invocation_id"
          ],
          "unique": true
        },
        {
          "name": "uk_evaluation_completions_run_id_gen_candidate_key",
          "columns": [
            "run_id",
            "gen_candidate_key"
          ],
          "unique": true
        },
        {
          "name": "uk_evaluation_completions_run_id_gen_case_key_gen_slot__2792b280",
          "columns": [
            "run_id",
            "gen_case_key",
            "gen_slot_key",
            "execution_ordinal"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [],
      "checks": [
        {
          "name": "ck_evaluation_completions_shape",
          "sql": "(kind='generation' AND case_id IS NOT NULL AND slot_ordinal IS NOT NULL AND result_json IS NULL) OR (kind='semantic' AND candidate_id IS NOT NULL AND case_id IS NULL AND slot_ordinal IS NULL AND candidate_json IS NULL)"
        },
        {
          "name": "ck_evaluation_completions_kind",
          "sql": "kind IN ('generation','semantic')"
        }
      ]
    },
    {
      "name": "evaluation_dispatches",
      "create_sql": "\nCREATE TABLE evaluation_dispatches (\n\trun_id CHAR(36) NOT NULL, \n\tinvocation_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\texecution_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tkind VARCHAR(16) NOT NULL, \n\tcase_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tslot_ordinal INTEGER NOT NULL, \n\tcandidate_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tcheckpoint_json JSON NOT NULL, \n\tCONSTRAINT pk_evaluation_dispatches PRIMARY KEY (run_id, invocation_id), \n\tCONSTRAINT uk_evaluation_dispatches_run_id_execution_id UNIQUE (run_id, execution_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "run_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "invocation_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "execution_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "kind",
          "type": "VARCHAR(16)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "case_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "slot_ordinal",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "candidate_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "checkpoint_json",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id",
        "invocation_id"
      ],
      "indexes": [
        {
          "name": "uk_evaluation_dispatches_run_id_execution_id",
          "columns": [
            "run_id",
            "execution_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "evaluation_runs",
      "create_sql": "\nCREATE TABLE evaluation_runs (\n\trun_id CHAR(36) NOT NULL, \n\texecution_mode VARCHAR(32) NOT NULL DEFAULT 'serial_v1', \n\torganization_id BIGINT NOT NULL, \n\trequested_by VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tdefinition_json LONGTEXT NOT NULL, \n\tprogress_json JSON, \n\tfrozen_execution_policy_fingerprint VARCHAR(71), \n\tfrozen_execution_policy_json LONGTEXT, \n\tCONSTRAINT pk_evaluation_runs PRIMARY KEY (run_id), \n\tCONSTRAINT ck_evaluation_runs_frozen_policy CHECK ((frozen_execution_policy_fingerprint IS NULL AND frozen_execution_policy_json IS NULL) OR (frozen_execution_policy_fingerprint IS NOT NULL AND frozen_execution_policy_json IS NOT NULL))\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_evaluation_runs_organization_id ON evaluation_runs (organization_id)"
      ],
      "columns": [
        {
          "name": "run_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "execution_mode",
          "type": "VARCHAR(32)",
          "nullable": false,
          "collation": null,
          "default": "serial_v1",
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "requested_by",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "definition_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "progress_json",
          "type": "JSON",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "frozen_execution_policy_fingerprint",
          "type": "VARCHAR(71)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "frozen_execution_policy_json",
          "type": "LONGTEXT",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id"
      ],
      "indexes": [
        {
          "name": "idx_evaluation_runs_organization_id",
          "columns": [
            "organization_id"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [],
      "checks": [
        {
          "name": "ck_evaluation_runs_frozen_policy",
          "sql": "(frozen_execution_policy_fingerprint IS NULL AND frozen_execution_policy_json IS NULL) OR (frozen_execution_policy_fingerprint IS NOT NULL AND frozen_execution_policy_json IS NOT NULL)"
        }
      ]
    },
    {
      "name": "evaluation_suites",
      "create_sql": "\nCREATE TABLE evaluation_suites (\n\tsuite_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tsuite_version VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tfingerprint VARCHAR(71) NOT NULL, \n\tdefinition_json LONGTEXT NOT NULL, \n\tcommand_id CHAR(36), \n\torganization_id BIGINT NOT NULL, \n\toperator_user_id BIGINT, \n\treceipt_json LONGTEXT, \n\treceipt_sha256 CHAR(64), \n\tsource_ref VARCHAR(255), \n\timported_by VARCHAR(128), \n\tcontracts_json LONGTEXT, \n\tcontracts_sha256 CHAR(64), \n\tCONSTRAINT pk_evaluation_suites PRIMARY KEY (suite_id, suite_version), \n\tCONSTRAINT uk_evaluation_suites_command_id UNIQUE (command_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "suite_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "suite_version",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "fingerprint",
          "type": "VARCHAR(71)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "definition_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "command_id",
          "type": "CHAR(36)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_json",
          "type": "LONGTEXT",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_sha256",
          "type": "CHAR(64)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "source_ref",
          "type": "VARCHAR(255)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "imported_by",
          "type": "VARCHAR(128)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "contracts_json",
          "type": "LONGTEXT",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "contracts_sha256",
          "type": "CHAR(64)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "suite_id",
        "suite_version"
      ],
      "indexes": [
        {
          "name": "uk_evaluation_suites_command_id",
          "columns": [
            "command_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "execution_leases",
      "create_sql": "\nCREATE TABLE execution_leases (\n\tthread_id VARCHAR(191) COLLATE utf8mb4_bin NOT NULL, \n\tfence BIGINT UNSIGNED NOT NULL, \n\texpires_at DATETIME(6) NOT NULL, \n\tCONSTRAINT pk_execution_leases PRIMARY KEY (thread_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "thread_id",
          "type": "VARCHAR(191) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "fence",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "expires_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "thread_id"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "execution_participant_retries",
      "create_sql": "\nCREATE TABLE execution_participant_retries (\n\torganization_id BIGINT UNSIGNED NOT NULL, \n\tcommand_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\trequest_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsource_run_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\trun_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\toperator_user_id BIGINT UNSIGNED NOT NULL, \n\texpected_version INTEGER NOT NULL, \n\treason TEXT NOT NULL, \n\taccepted_unknown_risk BOOL NOT NULL, \n\tsource_failure_code VARCHAR(64), \n\tfrozen_request_json LONGTEXT, \n\treceipt JSON NOT NULL, \n\tcreated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), \n\tCONSTRAINT pk_execution_participant_retries PRIMARY KEY (organization_id, command_id), \n\tCONSTRAINT uk_execution_participant_retries_run_id UNIQUE (run_id), \n\tCONSTRAINT uk_execution_participant_retries_source_run_id UNIQUE (source_run_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_execution_participant_retries_session_id ON execution_participant_retries (session_id)"
      ],
      "columns": [
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "command_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "source_run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "expected_version",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "reason",
          "type": "TEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "accepted_unknown_risk",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "source_failure_code",
          "type": "VARCHAR(64)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "frozen_request_json",
          "type": "LONGTEXT",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "organization_id",
        "command_id"
      ],
      "indexes": [
        {
          "name": "idx_execution_participant_retries_session_id",
          "columns": [
            "session_id"
          ],
          "unique": false
        },
        {
          "name": "uk_execution_participant_retries_run_id",
          "columns": [
            "run_id"
          ],
          "unique": true
        },
        {
          "name": "uk_execution_participant_retries_source_run_id",
          "columns": [
            "source_run_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "governance_asset_versions",
      "create_sql": "\nCREATE TABLE governance_asset_versions (\n\tasset_row_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, \n\tasset_kind VARCHAR(32) COLLATE ascii_bin NOT NULL, \n\towner_organization_id BIGINT UNSIGNED NOT NULL DEFAULT '0', \n\tasset_id VARCHAR(255) COLLATE utf8mb4_0900_bin NOT NULL, \n\tversion VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tfingerprint VARCHAR(71) COLLATE utf8mb4_bin NOT NULL, \n\tbody_format VARCHAR(32) COLLATE ascii_bin NOT NULL, \n\tbody_bytes LONGBLOB NOT NULL, \n\tpackage_sha256 VARCHAR(64) COLLATE utf8mb4_bin, \n\tsource_ref VARCHAR(255) COLLATE utf8mb4_bin NOT NULL, \n\timported_by VARCHAR(255) COLLATE utf8mb4_bin NOT NULL, \n\tcreated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), \n\tnative_id_key VARCHAR(255) COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS (CASE WHEN asset_kind IN ('profile','prompt','route','schema') THEN asset_id ELSE NULL END) STORED, \n\tscoped_id_key VARCHAR(128) COLLATE utf8mb4_bin GENERATED ALWAYS AS (CASE WHEN asset_kind IN ('execution_policy','gate_policy','semantic_prompt') THEN asset_id ELSE NULL END) STORED, \n\tprofile_id_key VARCHAR(255) COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS (CASE WHEN asset_kind='profile' THEN asset_id ELSE NULL END) STORED, \n\tcatalog_version_key VARCHAR(128) COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS (version) STORED, \n\tCONSTRAINT pk_governance_asset_versions PRIMARY KEY (asset_row_id), \n\tCONSTRAINT ck_governance_asset_versions_scope CHECK (asset_kind='semantic_prompt' OR owner_organization_id=0), \n\tCONSTRAINT ck_governance_asset_versions_shape CHECK ((asset_kind='prompt' AND body_format='prompt_package_json' AND package_sha256 IS NOT NULL) OR (asset_kind='semantic_prompt' AND body_format='semantic_markdown' AND package_sha256 IS NULL) OR (asset_kind IN ('profile','route','schema','execution_policy','gate_policy') AND body_format='definition_json' AND package_sha256 IS NULL)), \n\tCONSTRAINT uk_governance_asset_versions_profile_id_key_version UNIQUE (profile_id_key, version), \n\tCONSTRAINT ck_governance_asset_versions_time CHECK (asset_kind NOT IN ('execution_policy','gate_policy','semantic_prompt') OR created_at IS NOT NULL), \n\tCONSTRAINT ck_governance_asset_versions_kind CHECK (asset_kind IN ('profile','prompt','route','schema','execution_policy','gate_policy','semantic_prompt')), \n\tCONSTRAINT ck_governance_asset_versions_identity CHECK (asset_kind NOT IN ('execution_policy','gate_policy','semantic_prompt') OR CHAR_LENGTH(asset_id)<=128), \n\tCONSTRAINT uk_governance_asset_versions_asset_kind_owner_organizat_27e4f6ea UNIQUE (asset_kind, owner_organization_id, scoped_id_key, version), \n\tCONSTRAINT uk_governance_asset_versions_asset_kind_owner_organizat_82a62697 UNIQUE (asset_kind, owner_organization_id, native_id_key, version)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_governance_asset_versions_asset_kind_owner_organiza_e150eb87 ON governance_asset_versions (asset_kind, owner_organization_id, asset_id, catalog_version_key)"
      ],
      "columns": [
        {
          "name": "asset_row_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "asset_kind",
          "type": "VARCHAR(32) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "owner_organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": "0",
          "computed": null,
          "persisted": null
        },
        {
          "name": "asset_id",
          "type": "VARCHAR(255) COLLATE utf8mb4_0900_bin",
          "nullable": false,
          "collation": "utf8mb4_0900_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "version",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "fingerprint",
          "type": "VARCHAR(71) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "body_format",
          "type": "VARCHAR(32) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "body_bytes",
          "type": "LONGBLOB",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "package_sha256",
          "type": "VARCHAR(64) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "source_ref",
          "type": "VARCHAR(255) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "imported_by",
          "type": "VARCHAR(255) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        },
        {
          "name": "native_id_key",
          "type": "VARCHAR(255) COLLATE utf8mb4_0900_bin",
          "nullable": true,
          "collation": "utf8mb4_0900_bin",
          "default": null,
          "computed": "CASE WHEN asset_kind IN ('profile','prompt','route','schema') THEN asset_id ELSE NULL END",
          "persisted": true
        },
        {
          "name": "scoped_id_key",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": "CASE WHEN asset_kind IN ('execution_policy','gate_policy','semantic_prompt') THEN asset_id ELSE NULL END",
          "persisted": true
        },
        {
          "name": "profile_id_key",
          "type": "VARCHAR(255) COLLATE utf8mb4_0900_bin",
          "nullable": true,
          "collation": "utf8mb4_0900_bin",
          "default": null,
          "computed": "CASE WHEN asset_kind='profile' THEN asset_id ELSE NULL END",
          "persisted": true
        },
        {
          "name": "catalog_version_key",
          "type": "VARCHAR(128) COLLATE utf8mb4_0900_bin",
          "nullable": true,
          "collation": "utf8mb4_0900_bin",
          "default": null,
          "computed": "version",
          "persisted": true
        }
      ],
      "primary_key": [
        "asset_row_id"
      ],
      "indexes": [
        {
          "name": "idx_governance_asset_versions_asset_kind_owner_organiza_e150eb87",
          "columns": [
            "asset_kind",
            "owner_organization_id",
            "asset_id",
            "catalog_version_key"
          ],
          "unique": false
        },
        {
          "name": "uk_governance_asset_versions_asset_kind_owner_organizat_27e4f6ea",
          "columns": [
            "asset_kind",
            "owner_organization_id",
            "scoped_id_key",
            "version"
          ],
          "unique": true
        },
        {
          "name": "uk_governance_asset_versions_profile_id_key_version",
          "columns": [
            "profile_id_key",
            "version"
          ],
          "unique": true
        },
        {
          "name": "uk_governance_asset_versions_asset_kind_owner_organizat_82a62697",
          "columns": [
            "asset_kind",
            "owner_organization_id",
            "native_id_key",
            "version"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [],
      "checks": [
        {
          "name": "ck_governance_asset_versions_kind",
          "sql": "asset_kind IN ('profile','prompt','route','schema','execution_policy','gate_policy','semantic_prompt')"
        },
        {
          "name": "ck_governance_asset_versions_time",
          "sql": "asset_kind NOT IN ('execution_policy','gate_policy','semantic_prompt') OR created_at IS NOT NULL"
        },
        {
          "name": "ck_governance_asset_versions_identity",
          "sql": "asset_kind NOT IN ('execution_policy','gate_policy','semantic_prompt') OR CHAR_LENGTH(asset_id)<=128"
        },
        {
          "name": "ck_governance_asset_versions_scope",
          "sql": "asset_kind='semantic_prompt' OR owner_organization_id=0"
        },
        {
          "name": "ck_governance_asset_versions_shape",
          "sql": "(asset_kind='prompt' AND body_format='prompt_package_json' AND package_sha256 IS NOT NULL) OR (asset_kind='semantic_prompt' AND body_format='semantic_markdown' AND package_sha256 IS NULL) OR (asset_kind IN ('profile','route','schema','execution_policy','gate_policy') AND body_format='definition_json' AND package_sha256 IS NULL)"
        }
      ]
    },
    {
      "name": "governance_draft_heads",
      "create_sql": "\nCREATE TABLE governance_draft_heads (\n\tdraft_row_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, \n\tdraft_kind VARCHAR(16) COLLATE ascii_bin NOT NULL, \n\torganization_id BIGINT UNSIGNED NOT NULL, \n\tdraft_id CHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\trevision BIGINT NOT NULL, \n\tprompt_draft_id_key CHAR(36) COLLATE utf8mb4_0900_ai_ci GENERATED ALWAYS AS (CASE WHEN draft_kind='prompt' THEN draft_id ELSE NULL END) STORED, \n\tsemantic_draft_id_key CHAR(36) COLLATE utf8mb4_bin GENERATED ALWAYS AS (CASE WHEN draft_kind='semantic' THEN draft_id ELSE NULL END) STORED, \n\tCONSTRAINT pk_governance_draft_heads PRIMARY KEY (draft_row_id), \n\tCONSTRAINT uk_governance_draft_heads_draft_row_id_draft_kind_organ_0b517c32 UNIQUE (draft_row_id, draft_kind, organization_id), \n\tCONSTRAINT ck_governance_draft_heads_kind CHECK (draft_kind IN ('prompt','semantic')), \n\tCONSTRAINT ck_governance_draft_heads_prompt_signed_scope CHECK (draft_kind <> 'prompt' OR organization_id <= 9223372036854775807), \n\tCONSTRAINT uk_governance_draft_heads_organization_id_semantic_draft_id_key UNIQUE (organization_id, semantic_draft_id_key), \n\tCONSTRAINT uk_governance_draft_heads_prompt_draft_id_key UNIQUE (prompt_draft_id_key), \n\tCONSTRAINT ck_governance_draft_heads_revision CHECK (revision>=0)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "draft_row_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "draft_kind",
          "type": "VARCHAR(16) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "draft_id",
          "type": "CHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "revision",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "prompt_draft_id_key",
          "type": "CHAR(36) COLLATE utf8mb4_0900_ai_ci",
          "nullable": true,
          "collation": "utf8mb4_0900_ai_ci",
          "default": null,
          "computed": "CASE WHEN draft_kind='prompt' THEN draft_id ELSE NULL END",
          "persisted": true
        },
        {
          "name": "semantic_draft_id_key",
          "type": "CHAR(36) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": "CASE WHEN draft_kind='semantic' THEN draft_id ELSE NULL END",
          "persisted": true
        }
      ],
      "primary_key": [
        "draft_row_id"
      ],
      "indexes": [
        {
          "name": "uk_governance_draft_heads_organization_id_semantic_draft_id_key",
          "columns": [
            "organization_id",
            "semantic_draft_id_key"
          ],
          "unique": true
        },
        {
          "name": "uk_governance_draft_heads_draft_row_id_draft_kind_organ_0b517c32",
          "columns": [
            "draft_row_id",
            "draft_kind",
            "organization_id"
          ],
          "unique": true
        },
        {
          "name": "uk_governance_draft_heads_prompt_draft_id_key",
          "columns": [
            "prompt_draft_id_key"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [],
      "checks": [
        {
          "name": "ck_governance_draft_heads_revision",
          "sql": "revision>=0"
        },
        {
          "name": "ck_governance_draft_heads_kind",
          "sql": "draft_kind IN ('prompt','semantic')"
        },
        {
          "name": "ck_governance_draft_heads_prompt_signed_scope",
          "sql": "draft_kind <> 'prompt' OR organization_id <= 9223372036854775807"
        }
      ]
    },
    {
      "name": "interpretation_idempotency_requests",
      "create_sql": "\nCREATE TABLE interpretation_idempotency_requests (\n\tscope_hash VARCHAR(64) COLLATE utf8mb4_bin NOT NULL, \n\t`key` VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\trequest_hash VARCHAR(64) NOT NULL, \n\tresponse JSON, \n\tcreated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), \n\tCONSTRAINT pk_interpretation_idempotency_requests PRIMARY KEY (scope_hash, `key`)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "scope_hash",
          "type": "VARCHAR(64) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "key",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_hash",
          "type": "VARCHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "response",
          "type": "JSON",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "scope_hash",
        "key"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "interpretation_sessions",
      "create_sql": "\nCREATE TABLE interpretation_sessions (\n\tid VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\trequest_id VARCHAR(36) COLLATE utf8mb4_bin, \n\torg_id BIGINT UNSIGNED NOT NULL, \n\towner_subject_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\ttestee_id BIGINT UNSIGNED NOT NULL, \n\tassessment_ids JSON NOT NULL, \n\tgoal TEXT NOT NULL, \n\tstatus VARCHAR(32) NOT NULL, \n\tversion INTEGER NOT NULL, \n\tactive_run_id VARCHAR(36) COLLATE utf8mb4_bin, \n\tcurrent_question_id VARCHAR(36) COLLATE utf8mb4_bin, \n\tevidence_set_id VARCHAR(36) COLLATE utf8mb4_bin, \n\tworkflow_version VARCHAR(64) NOT NULL, \n\tfailure_code VARCHAR(64), \n\tcreated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), \n\tupdated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), \n\tcreated_at_utc DATETIME(6), \n\tupdated_at_utc DATETIME(6), \n\tCONSTRAINT pk_interpretation_sessions PRIMARY KEY (id), \n\tCONSTRAINT uk_interpretation_sessions_request_id UNIQUE (request_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_interpretation_sessions_org_id_owner_subject_id_upd_4309d3b9 ON interpretation_sessions (org_id, owner_subject_id, updated_at, id)"
      ],
      "columns": [
        {
          "name": "id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "org_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "owner_subject_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "testee_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "assessment_ids",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "goal",
          "type": "TEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "status",
          "type": "VARCHAR(32)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "version",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "active_run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "current_question_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "evidence_set_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "workflow_version",
          "type": "VARCHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "failure_code",
          "type": "VARCHAR(64)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        },
        {
          "name": "updated_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at_utc",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "updated_at_utc",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "id"
      ],
      "indexes": [
        {
          "name": "idx_interpretation_sessions_org_id_owner_subject_id_upd_4309d3b9",
          "columns": [
            "org_id",
            "owner_subject_id",
            "updated_at",
            "id"
          ],
          "unique": false
        },
        {
          "name": "uk_interpretation_sessions_request_id",
          "columns": [
            "request_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "messaging_evaluation_sequences",
      "create_sql": "\nCREATE TABLE messaging_evaluation_sequences (\n\trun_id CHAR(36) NOT NULL, \n\tsequence BIGINT NOT NULL, \n\tversion BIGINT NOT NULL, \n\tPRIMARY KEY (run_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "run_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "sequence",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "version",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "messaging_inbox",
      "create_sql": "\nCREATE TABLE messaging_inbox (\n\tproducer VARCHAR(64) COLLATE ascii_bin NOT NULL, \n\tmessage_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tdestination VARCHAR(64) COLLATE ascii_bin NOT NULL, \n\tbody_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, \n\tbody MEDIUMBLOB NOT NULL, \n\twire_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, \n\tkind INTEGER NOT NULL, \n\taggregate_key VARCHAR(192) COLLATE utf8mb4_bin NOT NULL, \n\treservation_token VARCHAR(36) NOT NULL, \n\tdecision VARCHAR(32) NOT NULL, \n\treceipt_id VARCHAR(128) COLLATE utf8mb4_bin, \n\treceived_at DATETIME(6) NOT NULL, \n\tPRIMARY KEY (producer, message_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "producer",
          "type": "VARCHAR(64) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "message_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "destination",
          "type": "VARCHAR(64) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "body_sha256",
          "type": "CHAR(64) CHARACTER SET ascii COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "body",
          "type": "MEDIUMBLOB",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "wire_sha256",
          "type": "CHAR(64) CHARACTER SET ascii COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "kind",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "aggregate_key",
          "type": "VARCHAR(192) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "reservation_token",
          "type": "VARCHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "decision",
          "type": "VARCHAR(32)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "received_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "producer",
        "message_id"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "messaging_observations",
      "create_sql": "\nCREATE TABLE messaging_observations (\n\tkind VARCHAR(64) COLLATE ascii_bin NOT NULL, \n\trecorded_count BIGINT UNSIGNED NOT NULL, \n\trecording_since DATETIME(6) NOT NULL, \n\tlast_observed_at DATETIME(6), \n\tPRIMARY KEY (kind)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "kind",
          "type": "VARCHAR(64) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "recorded_count",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "recording_since",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "last_observed_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "kind"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "messaging_outbox",
      "create_sql": "\nCREATE TABLE messaging_outbox (\n\tproducer VARCHAR(64) COLLATE ascii_bin NOT NULL, \n\tdestination VARCHAR(64) COLLATE ascii_bin NOT NULL, \n\tmessage_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tbody_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, \n\tbody MEDIUMBLOB NOT NULL, \n\twire MEDIUMBLOB NOT NULL, \n\twire_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, \n\ttopic VARCHAR(64) COLLATE ascii_bin NOT NULL, \n\tkind INTEGER NOT NULL, \n\torganization_id BIGINT UNSIGNED NOT NULL, \n\taggregate_key VARCHAR(192) COLLATE utf8mb4_bin NOT NULL, \n\taggregate_sequence BIGINT UNSIGNED NOT NULL, \n\tordered BOOL NOT NULL, \n\trequires_receipt BOOL NOT NULL, \n\tstage VARCHAR(32) COLLATE ascii_bin NOT NULL, \n\tattempts BIGINT UNSIGNED NOT NULL DEFAULT '0', \n\tavailable_at DATETIME(6) NOT NULL, \n\tcreated_at DATETIME(6) NOT NULL, \n\tpublished_at DATETIME(6), \n\tconfirmed_at DATETIME(6), \n\terror_code VARCHAR(128) COLLATE ascii_bin NOT NULL DEFAULT '', \n\tPRIMARY KEY (producer, destination, message_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_messaging_outbox_producer_destination_aggregate_key_ca00d30f ON messaging_outbox (producer, destination, aggregate_key, ordered, aggregate_sequence, stage)",
        "CREATE INDEX idx_messaging_outbox_stage_available_at_message_id ON messaging_outbox (stage, available_at, message_id)"
      ],
      "columns": [
        {
          "name": "producer",
          "type": "VARCHAR(64) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "destination",
          "type": "VARCHAR(64) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "message_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "body_sha256",
          "type": "CHAR(64) CHARACTER SET ascii COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "body",
          "type": "MEDIUMBLOB",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "wire",
          "type": "MEDIUMBLOB",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "wire_sha256",
          "type": "CHAR(64) CHARACTER SET ascii COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "topic",
          "type": "VARCHAR(64) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "kind",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "aggregate_key",
          "type": "VARCHAR(192) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "aggregate_sequence",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "ordered",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "requires_receipt",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "stage",
          "type": "VARCHAR(32) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "attempts",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": "0",
          "computed": null,
          "persisted": null
        },
        {
          "name": "available_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "published_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "confirmed_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "error_code",
          "type": "VARCHAR(128) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": "",
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "producer",
        "destination",
        "message_id"
      ],
      "indexes": [
        {
          "name": "idx_messaging_outbox_stage_available_at_message_id",
          "columns": [
            "stage",
            "available_at",
            "message_id"
          ],
          "unique": false
        },
        {
          "name": "idx_messaging_outbox_producer_destination_aggregate_key_ca00d30f",
          "columns": [
            "producer",
            "destination",
            "aggregate_key",
            "ordered",
            "aggregate_sequence",
            "stage"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "messaging_quarantine",
      "create_sql": "\nCREATE TABLE messaging_quarantine (\n\twire_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, \n\twire MEDIUMBLOB NOT NULL, \n\tcode VARCHAR(128) COLLATE ascii_bin NOT NULL, \n\tlogical_producer VARCHAR(64) COLLATE ascii_bin, \n\tlogical_message_id VARCHAR(128) COLLATE utf8mb4_bin, \n\tlogical_body_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin, \n\tattempts BIGINT UNSIGNED NOT NULL, \n\tfirst_seen_at DATETIME(6) NOT NULL, \n\tlast_seen_at DATETIME(6) NOT NULL, \n\tPRIMARY KEY (wire_sha256), \n\tCONSTRAINT uk_messaging_quarantine_logical_producer_logical_message_id UNIQUE (logical_producer, logical_message_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "wire_sha256",
          "type": "CHAR(64) CHARACTER SET ascii COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "wire",
          "type": "MEDIUMBLOB",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "code",
          "type": "VARCHAR(128) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "logical_producer",
          "type": "VARCHAR(64) COLLATE ascii_bin",
          "nullable": true,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "logical_message_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "logical_body_sha256",
          "type": "CHAR(64) CHARACTER SET ascii COLLATE ascii_bin",
          "nullable": true,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "attempts",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "first_seen_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "last_seen_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "wire_sha256"
      ],
      "indexes": [
        {
          "name": "uk_messaging_quarantine_logical_producer_logical_message_id",
          "columns": [
            "logical_producer",
            "logical_message_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "quota_evaluation_admission_locks",
      "create_sql": "\nCREATE TABLE quota_evaluation_admission_locks (\n\torganization_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, \n\tCONSTRAINT pk_quota_evaluation_admission_locks PRIMARY KEY (organization_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "organization_id"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "quota_evaluation_capacity_reservations",
      "create_sql": "\nCREATE TABLE quota_evaluation_capacity_reservations (\n\tquota_snapshot JSON, \n\trun_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\torganization_id BIGINT UNSIGNED NOT NULL, \n\tbudget_day DATE NOT NULL, \n\tprovider_calls INTEGER NOT NULL, \n\tdaily_limit INTEGER NOT NULL, \n\trequested_by VARCHAR(128) NOT NULL, \n\treserved_at DATETIME(6) NOT NULL, \n\tCONSTRAINT pk_quota_evaluation_capacity_reservations PRIMARY KEY (run_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_quota_evaluation_capacity_reservations_organization_761c2fd1 ON quota_evaluation_capacity_reservations (organization_id, budget_day)"
      ],
      "columns": [
        {
          "name": "quota_snapshot",
          "type": "JSON",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "budget_day",
          "type": "DATE",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "provider_calls",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "daily_limit",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "requested_by",
          "type": "VARCHAR(128)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "reserved_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id"
      ],
      "indexes": [
        {
          "name": "idx_quota_evaluation_capacity_reservations_organization_761c2fd1",
          "columns": [
            "organization_id",
            "budget_day"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "quota_organization_commands",
      "create_sql": "\nCREATE TABLE quota_organization_commands (\n\torganization_id BIGINT UNSIGNED NOT NULL, \n\tcommand_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\toperator_user_id BIGINT UNSIGNED NOT NULL, \n\trequest_json LONGTEXT NOT NULL, \n\treceipt_json LONGTEXT NOT NULL, \n\treceipt_sha256 VARCHAR(64) NOT NULL, \n\tCONSTRAINT pk_quota_organization_commands PRIMARY KEY (organization_id, command_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "command_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_sha256",
          "type": "VARCHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "organization_id",
        "command_id"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "quota_organization_versions",
      "create_sql": "\nCREATE TABLE quota_organization_versions (\n\torganization_id BIGINT UNSIGNED NOT NULL, \n\trevision BIGINT NOT NULL, \n\tdefinition_json LONGTEXT NOT NULL, \n\tdefinition_sha256 VARCHAR(64) NOT NULL, \n\toperator_user_id BIGINT UNSIGNED NOT NULL, \n\treason TEXT NOT NULL, \n\tcreated_at DATETIME(6) NOT NULL, \n\tCONSTRAINT pk_quota_organization_versions PRIMARY KEY (organization_id, revision)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "revision",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "definition_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "definition_sha256",
          "type": "VARCHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "reason",
          "type": "TEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "organization_id",
        "revision"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "quota_participant_admission_locks",
      "create_sql": "\nCREATE TABLE quota_participant_admission_locks (\n\torganization_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, \n\tCONSTRAINT pk_quota_participant_admission_locks PRIMARY KEY (organization_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "organization_id"
      ],
      "indexes": [],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "quota_participant_capacity_reservations",
      "create_sql": "\nCREATE TABLE quota_participant_capacity_reservations (\n\tquota_snapshot JSON, \n\trun_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\torganization_id BIGINT UNSIGNED NOT NULL, \n\tsubject_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tassessment_ids JSON NOT NULL, \n\tbudget_day DATE NOT NULL, \n\treserved_at DATETIME(6) NOT NULL, \n\tactive BOOL NOT NULL, \n\tacquired_at DATETIME(6), \n\treleased_at DATETIME(6), \n\tCONSTRAINT pk_quota_participant_capacity_reservations PRIMARY KEY (run_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_quota_participant_capacity_reservations_organizatio_d6561f2c ON quota_participant_capacity_reservations (organization_id, active)",
        "CREATE INDEX idx_quota_participant_capacity_reservations_organizatio_d9e18f4a ON quota_participant_capacity_reservations (organization_id, budget_day)"
      ],
      "columns": [
        {
          "name": "quota_snapshot",
          "type": "JSON",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "subject_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "assessment_ids",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "budget_day",
          "type": "DATE",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "reserved_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "active",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "acquired_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "released_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id"
      ],
      "indexes": [
        {
          "name": "idx_quota_participant_capacity_reservations_organizatio_d9e18f4a",
          "columns": [
            "organization_id",
            "budget_day"
          ],
          "unique": false
        },
        {
          "name": "idx_quota_participant_capacity_reservations_organizatio_d6561f2c",
          "columns": [
            "organization_id",
            "active"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [],
      "checks": []
    },
    {
      "name": "evaluation_response_receipts",
      "create_sql": "\nCREATE TABLE evaluation_response_receipts (\n\trun_id CHAR(36) NOT NULL, \n\tinvocation_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\texecution_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tclaim_version BIGINT NOT NULL, \n\tdefinition_json LONGTEXT NOT NULL, \n\tsha256 CHAR(64) NOT NULL, \n\tCONSTRAINT pk_evaluation_response_receipts PRIMARY KEY (run_id, invocation_id), \n\tCONSTRAINT uk_evaluation_response_receipts_run_id_execution_id UNIQUE (run_id, execution_id), \n\tCONSTRAINT fk_evaluation_response_receipts_run_id FOREIGN KEY(run_id) REFERENCES evaluation_runs (run_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "run_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "invocation_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "execution_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "claim_version",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "definition_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "sha256",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id",
        "invocation_id"
      ],
      "indexes": [
        {
          "name": "uk_evaluation_response_receipts_run_id_execution_id",
          "columns": [
            "run_id",
            "execution_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_evaluation_response_receipts_run_id",
          "columns": [
            "run_id"
          ],
          "table": "evaluation_runs",
          "referred_columns": [
            "run_id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "evaluation_slot_claims",
      "create_sql": "\nCREATE TABLE evaluation_slot_claims (\n\trun_id CHAR(36) NOT NULL, \n\tcase_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\tslot_ordinal INTEGER NOT NULL, \n\tversion BIGINT NOT NULL, \n\tcheckpoint_json JSON NOT NULL, \n\tCONSTRAINT pk_evaluation_slot_claims PRIMARY KEY (run_id, case_id, slot_ordinal), \n\tCONSTRAINT fk_evaluation_slot_claims_run_id FOREIGN KEY(run_id) REFERENCES evaluation_runs (run_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "run_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "case_id",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "slot_ordinal",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "version",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "checkpoint_json",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id",
        "case_id",
        "slot_ordinal"
      ],
      "indexes": [],
      "foreign_keys": [
        {
          "name": "fk_evaluation_slot_claims_run_id",
          "columns": [
            "run_id"
          ],
          "table": "evaluation_runs",
          "referred_columns": [
            "run_id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "governance_draft_versions",
      "create_sql": "\nCREATE TABLE governance_draft_versions (\n\tdraft_row_id BIGINT UNSIGNED NOT NULL, \n\trevision BIGINT NOT NULL, \n\tdraft_kind VARCHAR(16) COLLATE ascii_bin NOT NULL, \n\torganization_id BIGINT UNSIGNED NOT NULL, \n\tsnapshot_bytes LONGBLOB NOT NULL, \n\tsnapshot_sha256 CHAR(64) NOT NULL, \n\tcommand_id CHAR(36) COLLATE utf8mb4_0900_ai_ci, \n\toperator_user_id BIGINT UNSIGNED, \n\trequest_bytes LONGBLOB, \n\tprompt_command_id_key CHAR(36) COLLATE utf8mb4_0900_ai_ci GENERATED ALWAYS AS (CASE WHEN draft_kind='prompt' THEN command_id ELSE NULL END) STORED, \n\tCONSTRAINT pk_governance_draft_versions PRIMARY KEY (draft_row_id, revision), \n\tCONSTRAINT fk_governance_draft_versions_draft_row_id_draft_kind_or_9ed91db7 FOREIGN KEY(draft_row_id, draft_kind, organization_id) REFERENCES governance_draft_heads (draft_row_id, draft_kind, organization_id), \n\tCONSTRAINT ck_governance_draft_versions_audit CHECK ((draft_kind='prompt' AND command_id IS NOT NULL AND operator_user_id IS NOT NULL AND request_bytes IS NOT NULL) OR (draft_kind='semantic' AND command_id IS NULL AND operator_user_id IS NULL AND request_bytes IS NULL)), \n\tCONSTRAINT uk_governance_draft_versions_prompt_command_id_key UNIQUE (prompt_command_id_key), \n\tCONSTRAINT ck_governance_draft_versions_revision CHECK (revision>0), \n\tCONSTRAINT ck_governance_draft_versions_kind CHECK (draft_kind IN ('prompt','semantic')), \n\tCONSTRAINT ck_governance_draft_versions_prompt_signed_audit CHECK (draft_kind <> 'prompt' OR (organization_id <= 9223372036854775807 AND operator_user_id <= 9223372036854775807))\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_governance_draft_versions_draft_row_id_draft_kind_o_0874c6c9 ON governance_draft_versions (draft_row_id, draft_kind, organization_id)"
      ],
      "columns": [
        {
          "name": "draft_row_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "revision",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "draft_kind",
          "type": "VARCHAR(16) COLLATE ascii_bin",
          "nullable": false,
          "collation": "ascii_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "snapshot_bytes",
          "type": "LONGBLOB",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "snapshot_sha256",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "command_id",
          "type": "CHAR(36) COLLATE utf8mb4_0900_ai_ci",
          "nullable": true,
          "collation": "utf8mb4_0900_ai_ci",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT UNSIGNED",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_bytes",
          "type": "LONGBLOB",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "prompt_command_id_key",
          "type": "CHAR(36) COLLATE utf8mb4_0900_ai_ci",
          "nullable": true,
          "collation": "utf8mb4_0900_ai_ci",
          "default": null,
          "computed": "CASE WHEN draft_kind='prompt' THEN command_id ELSE NULL END",
          "persisted": true
        }
      ],
      "primary_key": [
        "draft_row_id",
        "revision"
      ],
      "indexes": [
        {
          "name": "idx_governance_draft_versions_draft_row_id_draft_kind_o_0874c6c9",
          "columns": [
            "draft_row_id",
            "draft_kind",
            "organization_id"
          ],
          "unique": false
        },
        {
          "name": "uk_governance_draft_versions_prompt_command_id_key",
          "columns": [
            "prompt_command_id_key"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_governance_draft_versions_draft_row_id_draft_kind_or_9ed91db7",
          "columns": [
            "draft_row_id",
            "draft_kind",
            "organization_id"
          ],
          "table": "governance_draft_heads",
          "referred_columns": [
            "draft_row_id",
            "draft_kind",
            "organization_id"
          ],
          "ondelete": null
        }
      ],
      "checks": [
        {
          "name": "ck_governance_draft_versions_revision",
          "sql": "revision>0"
        },
        {
          "name": "ck_governance_draft_versions_kind",
          "sql": "draft_kind IN ('prompt','semantic')"
        },
        {
          "name": "ck_governance_draft_versions_prompt_signed_audit",
          "sql": "draft_kind <> 'prompt' OR (organization_id <= 9223372036854775807 AND operator_user_id <= 9223372036854775807)"
        },
        {
          "name": "ck_governance_draft_versions_audit",
          "sql": "(draft_kind='prompt' AND command_id IS NOT NULL AND operator_user_id IS NOT NULL AND request_bytes IS NOT NULL) OR (draft_kind='semantic' AND command_id IS NULL AND operator_user_id IS NULL AND request_bytes IS NULL)"
        }
      ]
    },
    {
      "name": "governance_profile_registrations",
      "create_sql": "\nCREATE TABLE governance_profile_registrations (\n\tcommand_id CHAR(36) NOT NULL, \n\torganization_id BIGINT NOT NULL, \n\toperator_user_id BIGINT NOT NULL, \n\tprofile_id VARCHAR(255) COLLATE utf8mb4_0900_bin NOT NULL, \n\tprofile_version VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\treceipt_json LONGTEXT NOT NULL, \n\treceipt_sha256 CHAR(64) NOT NULL, \n\tCONSTRAINT pk_governance_profile_registrations PRIMARY KEY (command_id), \n\tCONSTRAINT uk_governance_profile_registrations_profile_id_profile_version UNIQUE (profile_id, profile_version), \n\tCONSTRAINT fk_governance_profile_registrations_profile_id_profile_version FOREIGN KEY(profile_id, profile_version) REFERENCES governance_asset_versions (profile_id_key, version)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "command_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "profile_id",
          "type": "VARCHAR(255) COLLATE utf8mb4_0900_bin",
          "nullable": false,
          "collation": "utf8mb4_0900_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "profile_version",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_sha256",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "command_id"
      ],
      "indexes": [
        {
          "name": "uk_governance_profile_registrations_profile_id_profile_version",
          "columns": [
            "profile_id",
            "profile_version"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_governance_profile_registrations_profile_id_profile_version",
          "columns": [
            "profile_id",
            "profile_version"
          ],
          "table": "governance_asset_versions",
          "referred_columns": [
            "profile_id_key",
            "version"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "governance_prompt_draft_freezes",
      "create_sql": "\nCREATE TABLE governance_prompt_draft_freezes (\n\tcommand_id CHAR(36) NOT NULL, \n\tdraft_id CHAR(36) COLLATE utf8mb4_0900_ai_ci NOT NULL, \n\torganization_id BIGINT NOT NULL, \n\toperator_user_id BIGINT NOT NULL, \n\treceipt_json LONGTEXT NOT NULL, \n\treceipt_sha256 CHAR(64) NOT NULL, \n\tCONSTRAINT pk_governance_prompt_draft_freezes PRIMARY KEY (command_id), \n\tCONSTRAINT fk_governance_prompt_draft_freezes_draft_id FOREIGN KEY(draft_id) REFERENCES governance_draft_heads (prompt_draft_id_key), \n\tCONSTRAINT uk_governance_prompt_draft_freezes_draft_id UNIQUE (draft_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "command_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "draft_id",
          "type": "CHAR(36) COLLATE utf8mb4_0900_ai_ci",
          "nullable": false,
          "collation": "utf8mb4_0900_ai_ci",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_sha256",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "command_id"
      ],
      "indexes": [
        {
          "name": "uk_governance_prompt_draft_freezes_draft_id",
          "columns": [
            "draft_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_governance_prompt_draft_freezes_draft_id",
          "columns": [
            "draft_id"
          ],
          "table": "governance_draft_heads",
          "referred_columns": [
            "prompt_draft_id_key"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "governance_semantic_draft_commands",
      "create_sql": "\nCREATE TABLE governance_semantic_draft_commands (\n\torganization_id BIGINT UNSIGNED NOT NULL, \n\tcommand_id CHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\toperator_user_id BIGINT UNSIGNED NOT NULL, \n\tdraft_id CHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\trequest_json LONGTEXT NOT NULL, \n\treceipt_json LONGTEXT NOT NULL, \n\treceipt_sha256 CHAR(64) NOT NULL, \n\tCONSTRAINT pk_governance_semantic_draft_commands PRIMARY KEY (organization_id, command_id), \n\tCONSTRAINT fk_governance_semantic_draft_commands_organization_id_draft_id FOREIGN KEY(organization_id, draft_id) REFERENCES governance_draft_heads (organization_id, semantic_draft_id_key)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_governance_semantic_draft_commands_organization_id_draft_id ON governance_semantic_draft_commands (organization_id, draft_id)"
      ],
      "columns": [
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "command_id",
          "type": "CHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "draft_id",
          "type": "CHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_sha256",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "organization_id",
        "command_id"
      ],
      "indexes": [
        {
          "name": "idx_governance_semantic_draft_commands_organization_id_draft_id",
          "columns": [
            "organization_id",
            "draft_id"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_governance_semantic_draft_commands_organization_id_draft_id",
          "columns": [
            "organization_id",
            "draft_id"
          ],
          "table": "governance_draft_heads",
          "referred_columns": [
            "organization_id",
            "semantic_draft_id_key"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "governance_solutions",
      "create_sql": "\nCREATE TABLE governance_solutions (\n\tsolution_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\torganization_id BIGINT UNSIGNED NOT NULL, \n\trevision BIGINT NOT NULL, \n\tdraft_id CHAR(36) COLLATE utf8mb4_0900_ai_ci NOT NULL, \n\tstate_json LONGTEXT NOT NULL, \n\tstate_sha256 VARCHAR(64) NOT NULL, \n\tCONSTRAINT pk_governance_solutions PRIMARY KEY (solution_id), \n\tCONSTRAINT fk_governance_solutions_draft_id FOREIGN KEY(draft_id) REFERENCES governance_draft_heads (prompt_draft_id_key), \n\tCONSTRAINT uk_governance_solutions_draft_id UNIQUE (draft_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_governance_solutions_organization_id_solution_id ON governance_solutions (organization_id, solution_id)"
      ],
      "columns": [
        {
          "name": "solution_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "revision",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "draft_id",
          "type": "CHAR(36) COLLATE utf8mb4_0900_ai_ci",
          "nullable": false,
          "collation": "utf8mb4_0900_ai_ci",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "state_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "state_sha256",
          "type": "VARCHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "solution_id"
      ],
      "indexes": [
        {
          "name": "idx_governance_solutions_organization_id_solution_id",
          "columns": [
            "organization_id",
            "solution_id"
          ],
          "unique": false
        },
        {
          "name": "uk_governance_solutions_draft_id",
          "columns": [
            "draft_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_governance_solutions_draft_id",
          "columns": [
            "draft_id"
          ],
          "table": "governance_draft_heads",
          "referred_columns": [
            "prompt_draft_id_key"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "interpretation_clarifications",
      "create_sql": "\nCREATE TABLE interpretation_clarifications (\n\tid VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tquestion_seq INTEGER NOT NULL, \n\ttext TEXT NOT NULL, \n\tcan_skip BOOL NOT NULL, \n\tanswer TEXT, \n\tskipped BOOL NOT NULL, \n\tanswered_by VARCHAR(128), \n\tanswered_at DATETIME(6), \n\tCONSTRAINT pk_interpretation_clarifications PRIMARY KEY (id), \n\tCONSTRAINT fk_interpretation_clarifications_session_id FOREIGN KEY(session_id) REFERENCES interpretation_sessions (id), \n\tCONSTRAINT uk_interpretation_clarifications_session_id_question_seq UNIQUE (session_id, question_seq)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "question_seq",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "text",
          "type": "TEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "can_skip",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "answer",
          "type": "TEXT",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "skipped",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "answered_by",
          "type": "VARCHAR(128)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "answered_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "id"
      ],
      "indexes": [
        {
          "name": "uk_interpretation_clarifications_session_id_question_seq",
          "columns": [
            "session_id",
            "question_seq"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_interpretation_clarifications_session_id",
          "columns": [
            "session_id"
          ],
          "table": "interpretation_sessions",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "interpretation_evidence_sets",
      "create_sql": "\nCREATE TABLE interpretation_evidence_sets (\n\tid VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tfingerprint VARCHAR(64) NOT NULL, \n\tschema_version VARCHAR(32) NOT NULL, \n\titems JSON NOT NULL, \n\tfrozen_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), \n\tCONSTRAINT pk_interpretation_evidence_sets PRIMARY KEY (id), \n\tCONSTRAINT fk_interpretation_evidence_sets_session_id FOREIGN KEY(session_id) REFERENCES interpretation_sessions (id), \n\tCONSTRAINT uk_interpretation_evidence_sets_session_id UNIQUE (session_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "fingerprint",
          "type": "VARCHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "schema_version",
          "type": "VARCHAR(32)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "items",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "frozen_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "id"
      ],
      "indexes": [
        {
          "name": "uk_interpretation_evidence_sets_session_id",
          "columns": [
            "session_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_interpretation_evidence_sets_session_id",
          "columns": [
            "session_id"
          ],
          "table": "interpretation_sessions",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "interpretation_result_outbox",
      "create_sql": "\nCREATE TABLE interpretation_result_outbox (\n\tevent_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tversion INTEGER NOT NULL, \n\tpayload JSON NOT NULL, \n\tdelivered BOOL NOT NULL DEFAULT 0, \n\tmq_owned BOOL NOT NULL DEFAULT 0, \n\tattempts INTEGER NOT NULL DEFAULT 0, \n\tcreated_at DATETIME(6), \n\tdelivered_at DATETIME(6), \n\tavailable_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), \n\tCONSTRAINT pk_interpretation_result_outbox PRIMARY KEY (event_id), \n\tCONSTRAINT uk_interpretation_result_outbox_session_id_version UNIQUE (session_id, version), \n\tCONSTRAINT fk_interpretation_result_outbox_session_id FOREIGN KEY(session_id) REFERENCES interpretation_sessions (id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_interpretation_result_outbox_delivered_available_at_event_id ON interpretation_result_outbox (delivered, available_at, event_id)"
      ],
      "columns": [
        {
          "name": "event_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "version",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "payload",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "delivered",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": "0",
          "computed": null,
          "persisted": null
        },
        {
          "name": "mq_owned",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": "0",
          "computed": null,
          "persisted": null
        },
        {
          "name": "attempts",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": "0",
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "delivered_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "available_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "event_id"
      ],
      "indexes": [
        {
          "name": "idx_interpretation_result_outbox_delivered_available_at_event_id",
          "columns": [
            "delivered",
            "available_at",
            "event_id"
          ],
          "unique": false
        },
        {
          "name": "uk_interpretation_result_outbox_session_id_version",
          "columns": [
            "session_id",
            "version"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_interpretation_result_outbox_session_id",
          "columns": [
            "session_id"
          ],
          "table": "interpretation_sessions",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "interpretation_runs",
      "create_sql": "\nCREATE TABLE interpretation_runs (\n\tid VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_version INTEGER NOT NULL, \n\tstatus VARCHAR(32) NOT NULL, \n\tcheckpoint_ref VARCHAR(128), \n\tCONSTRAINT pk_interpretation_runs PRIMARY KEY (id), \n\tCONSTRAINT fk_interpretation_runs_session_id FOREIGN KEY(session_id) REFERENCES interpretation_sessions (id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_interpretation_runs_session_id ON interpretation_runs (session_id)"
      ],
      "columns": [
        {
          "name": "id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_version",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "status",
          "type": "VARCHAR(32)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "checkpoint_ref",
          "type": "VARCHAR(128)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "id"
      ],
      "indexes": [
        {
          "name": "idx_interpretation_runs_session_id",
          "columns": [
            "session_id"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_interpretation_runs_session_id",
          "columns": [
            "session_id"
          ],
          "table": "interpretation_sessions",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "operations_runtime_milestones",
      "create_sql": "\nCREATE TABLE operations_runtime_milestones (\n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tdedupe_key VARCHAR(128) COLLATE utf8mb4_bin NOT NULL, \n\trun_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tkind VARCHAR(32) NOT NULL, \n\tinvocation_id VARCHAR(36) COLLATE utf8mb4_bin, \n\tattempt INTEGER, \n\toccurred_at DATETIME(6) NOT NULL, \n\texpires_at DATETIME(6) NOT NULL, \n\tCONSTRAINT pk_operations_runtime_milestones PRIMARY KEY (session_id, dedupe_key), \n\tCONSTRAINT fk_operations_runtime_milestones_session_id FOREIGN KEY(session_id) REFERENCES interpretation_sessions (id) ON DELETE CASCADE\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_operations_runtime_milestones_expires_at ON operations_runtime_milestones (expires_at)"
      ],
      "columns": [
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "dedupe_key",
          "type": "VARCHAR(128) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "kind",
          "type": "VARCHAR(32)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "invocation_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "attempt",
          "type": "INTEGER",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "occurred_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "expires_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "session_id",
        "dedupe_key"
      ],
      "indexes": [
        {
          "name": "idx_operations_runtime_milestones_expires_at",
          "columns": [
            "expires_at"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_operations_runtime_milestones_session_id",
          "columns": [
            "session_id"
          ],
          "table": "interpretation_sessions",
          "referred_columns": [
            "id"
          ],
          "ondelete": "CASCADE"
        }
      ],
      "checks": []
    },
    {
      "name": "publication_records",
      "create_sql": "\nCREATE TABLE publication_records (\n\tpublication_id CHAR(36) NOT NULL, \n\tselector_key CHAR(64) NOT NULL, \n\trun_id CHAR(36) NOT NULL, \n\trun_version BIGINT NOT NULL, \n\torganization_id BIGINT NOT NULL, \n\tcontent_json LONGTEXT NOT NULL, \n\tcontent_sha256 CHAR(64) NOT NULL, \n\tCONSTRAINT pk_publication_records PRIMARY KEY (publication_id), \n\tCONSTRAINT fk_publication_records_run_id FOREIGN KEY(run_id) REFERENCES evaluation_runs (run_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_publication_records_run_id ON publication_records (run_id)"
      ],
      "columns": [
        {
          "name": "publication_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "selector_key",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_version",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "content_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "content_sha256",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "publication_id"
      ],
      "indexes": [
        {
          "name": "idx_publication_records_run_id",
          "columns": [
            "run_id"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_publication_records_run_id",
          "columns": [
            "run_id"
          ],
          "table": "evaluation_runs",
          "referred_columns": [
            "run_id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "quota_organization_pointers",
      "create_sql": "\nCREATE TABLE quota_organization_pointers (\n\torganization_id BIGINT UNSIGNED NOT NULL, \n\trevision BIGINT NOT NULL, \n\tCONSTRAINT pk_quota_organization_pointers PRIMARY KEY (organization_id), \n\tCONSTRAINT fk_quota_organization_pointers_organization_id_revision FOREIGN KEY(organization_id, revision) REFERENCES quota_organization_versions (organization_id, revision)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_quota_organization_pointers_organization_id_revision ON quota_organization_pointers (organization_id, revision)"
      ],
      "columns": [
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "revision",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "organization_id"
      ],
      "indexes": [
        {
          "name": "idx_quota_organization_pointers_organization_id_revision",
          "columns": [
            "organization_id",
            "revision"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_quota_organization_pointers_organization_id_revision",
          "columns": [
            "organization_id",
            "revision"
          ],
          "table": "quota_organization_versions",
          "referred_columns": [
            "organization_id",
            "revision"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "execution_configurations",
      "create_sql": "\nCREATE TABLE execution_configurations (\n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tevidence_set_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tevidence_fingerprint CHAR(64) NOT NULL, \n\tpublication_id CHAR(36) NOT NULL, \n\tpublication_sha256 CHAR(64) NOT NULL, \n\tpointer_version BIGINT NOT NULL, \n\tselector_query TEXT NOT NULL, \n\tCONSTRAINT pk_execution_configurations PRIMARY KEY (session_id), \n\tCONSTRAINT fk_execution_configurations_publication_id FOREIGN KEY(publication_id) REFERENCES publication_records (publication_id), \n\tCONSTRAINT fk_execution_configurations_evidence_set_id FOREIGN KEY(evidence_set_id) REFERENCES interpretation_evidence_sets (id), \n\tCONSTRAINT fk_execution_configurations_session_id FOREIGN KEY(session_id) REFERENCES interpretation_sessions (id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_execution_configurations_evidence_set_id ON execution_configurations (evidence_set_id)",
        "CREATE INDEX idx_execution_configurations_publication_id ON execution_configurations (publication_id)"
      ],
      "columns": [
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "evidence_set_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "evidence_fingerprint",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "publication_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "publication_sha256",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "pointer_version",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "selector_query",
          "type": "TEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "session_id"
      ],
      "indexes": [
        {
          "name": "idx_execution_configurations_evidence_set_id",
          "columns": [
            "evidence_set_id"
          ],
          "unique": false
        },
        {
          "name": "idx_execution_configurations_publication_id",
          "columns": [
            "publication_id"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_execution_configurations_session_id",
          "columns": [
            "session_id"
          ],
          "table": "interpretation_sessions",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        },
        {
          "name": "fk_execution_configurations_evidence_set_id",
          "columns": [
            "evidence_set_id"
          ],
          "table": "interpretation_evidence_sets",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        },
        {
          "name": "fk_execution_configurations_publication_id",
          "columns": [
            "publication_id"
          ],
          "table": "publication_records",
          "referred_columns": [
            "publication_id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "execution_jobs",
      "create_sql": "\nCREATE TABLE execution_jobs (\n\tid VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\trun_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tstatus VARCHAR(16) NOT NULL, \n\tavailable_at DATETIME(6) NOT NULL, \n\tlease_until DATETIME(6), \n\tfence_token BIGINT UNSIGNED NOT NULL, \n\tattempt INTEGER NOT NULL, \n\tanswer TEXT, \n\tskipped BOOL NOT NULL, \n\tquestion_id VARCHAR(36) COLLATE utf8mb4_bin, \n\tCONSTRAINT pk_execution_jobs PRIMARY KEY (id), \n\tCONSTRAINT fk_execution_jobs_session_id FOREIGN KEY(session_id) REFERENCES interpretation_sessions (id), \n\tCONSTRAINT uk_execution_jobs_run_id UNIQUE (run_id), \n\tCONSTRAINT fk_execution_jobs_run_id FOREIGN KEY(run_id) REFERENCES interpretation_runs (id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_execution_jobs_session_id ON execution_jobs (session_id)",
        "CREATE INDEX idx_execution_jobs_status_available_at_id ON execution_jobs (status, available_at, id)"
      ],
      "columns": [
        {
          "name": "id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "status",
          "type": "VARCHAR(16)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "available_at",
          "type": "DATETIME(6)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "lease_until",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "fence_token",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "attempt",
          "type": "INTEGER",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "answer",
          "type": "TEXT",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "skipped",
          "type": "BOOL",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "question_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": true,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "id"
      ],
      "indexes": [
        {
          "name": "idx_execution_jobs_status_available_at_id",
          "columns": [
            "status",
            "available_at",
            "id"
          ],
          "unique": false
        },
        {
          "name": "idx_execution_jobs_session_id",
          "columns": [
            "session_id"
          ],
          "unique": false
        },
        {
          "name": "uk_execution_jobs_run_id",
          "columns": [
            "run_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_execution_jobs_run_id",
          "columns": [
            "run_id"
          ],
          "table": "interpretation_runs",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        },
        {
          "name": "fk_execution_jobs_session_id",
          "columns": [
            "session_id"
          ],
          "table": "interpretation_sessions",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "execution_model_calls",
      "create_sql": "\nCREATE TABLE execution_model_calls (\n\trun_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tinvocation_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tfence_token BIGINT UNSIGNED NOT NULL, \n\tstatus VARCHAR(32) NOT NULL, \n\trequest_json LONGTEXT NOT NULL, \n\tresponse_json LONGTEXT, \n\tfailure_code VARCHAR(64), \n\tcreated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), \n\tcreated_at_utc DATETIME(6), \n\tCONSTRAINT pk_execution_model_calls PRIMARY KEY (run_id), \n\tCONSTRAINT fk_execution_model_calls_run_id FOREIGN KEY(run_id) REFERENCES interpretation_runs (id), \n\tCONSTRAINT uk_execution_model_calls_invocation_id UNIQUE (invocation_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "invocation_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "fence_token",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "status",
          "type": "VARCHAR(32)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "response_json",
          "type": "LONGTEXT",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "failure_code",
          "type": "VARCHAR(64)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at_utc",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "run_id"
      ],
      "indexes": [
        {
          "name": "uk_execution_model_calls_invocation_id",
          "columns": [
            "invocation_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_execution_model_calls_run_id",
          "columns": [
            "run_id"
          ],
          "table": "interpretation_runs",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "governance_solution_commands",
      "create_sql": "\nCREATE TABLE governance_solution_commands (\n\tcommand_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsolution_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\torganization_id BIGINT UNSIGNED NOT NULL, \n\toperator_user_id BIGINT UNSIGNED NOT NULL, \n\trequest_json LONGTEXT NOT NULL, \n\treceipt_json LONGTEXT NOT NULL, \n\treceipt_sha256 VARCHAR(64) NOT NULL, \n\tCONSTRAINT pk_governance_solution_commands PRIMARY KEY (command_id), \n\tCONSTRAINT fk_governance_solution_commands_solution_id FOREIGN KEY(solution_id) REFERENCES governance_solutions (solution_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_governance_solution_commands_solution_id ON governance_solution_commands (solution_id)"
      ],
      "columns": [
        {
          "name": "command_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "solution_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT UNSIGNED",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_json",
          "type": "LONGTEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_sha256",
          "type": "VARCHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "command_id"
      ],
      "indexes": [
        {
          "name": "idx_governance_solution_commands_solution_id",
          "columns": [
            "solution_id"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_governance_solution_commands_solution_id",
          "columns": [
            "solution_id"
          ],
          "table": "governance_solutions",
          "referred_columns": [
            "solution_id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "interpretation_artifacts",
      "create_sql": "\nCREATE TABLE interpretation_artifacts (\n\tid VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tsession_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\trun_id VARCHAR(36) COLLATE utf8mb4_bin NOT NULL, \n\tpayload JSON NOT NULL, \n\tcreated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), \n\tCONSTRAINT pk_interpretation_artifacts PRIMARY KEY (id), \n\tCONSTRAINT uk_interpretation_artifacts_run_id UNIQUE (run_id), \n\tCONSTRAINT uk_interpretation_artifacts_session_id UNIQUE (session_id), \n\tCONSTRAINT fk_interpretation_artifacts_run_id FOREIGN KEY(run_id) REFERENCES interpretation_runs (id), \n\tCONSTRAINT fk_interpretation_artifacts_session_id FOREIGN KEY(session_id) REFERENCES interpretation_sessions (id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "session_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "run_id",
          "type": "VARCHAR(36) COLLATE utf8mb4_bin",
          "nullable": false,
          "collation": "utf8mb4_bin",
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "payload",
          "type": "JSON",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "created_at",
          "type": "DATETIME(6)",
          "nullable": true,
          "collation": null,
          "default": "CURRENT_TIMESTAMP(6)",
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "id"
      ],
      "indexes": [
        {
          "name": "uk_interpretation_artifacts_run_id",
          "columns": [
            "run_id"
          ],
          "unique": true
        },
        {
          "name": "uk_interpretation_artifacts_session_id",
          "columns": [
            "session_id"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_interpretation_artifacts_run_id",
          "columns": [
            "run_id"
          ],
          "table": "interpretation_runs",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        },
        {
          "name": "fk_interpretation_artifacts_session_id",
          "columns": [
            "session_id"
          ],
          "table": "interpretation_sessions",
          "referred_columns": [
            "id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "publication_pointers",
      "create_sql": "\nCREATE TABLE publication_pointers (\n\tselector_key CHAR(64) NOT NULL, \n\tselector_json TEXT NOT NULL, \n\tversion BIGINT NOT NULL, \n\tactive_publication_id CHAR(36), \n\tchanged_at VARCHAR(64), \n\tCONSTRAINT pk_publication_pointers PRIMARY KEY (selector_key), \n\tCONSTRAINT fk_publication_pointers_active_publication_id FOREIGN KEY(active_publication_id) REFERENCES publication_records (publication_id)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [
        "CREATE INDEX idx_publication_pointers_active_publication_id ON publication_pointers (active_publication_id)"
      ],
      "columns": [
        {
          "name": "selector_key",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "selector_json",
          "type": "TEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "version",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "active_publication_id",
          "type": "CHAR(36)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "changed_at",
          "type": "VARCHAR(64)",
          "nullable": true,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "selector_key"
      ],
      "indexes": [
        {
          "name": "idx_publication_pointers_active_publication_id",
          "columns": [
            "active_publication_id"
          ],
          "unique": false
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_publication_pointers_active_publication_id",
          "columns": [
            "active_publication_id"
          ],
          "table": "publication_records",
          "referred_columns": [
            "publication_id"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    },
    {
      "name": "publication_changes",
      "create_sql": "\nCREATE TABLE publication_changes (\n\tcommand_id CHAR(36) NOT NULL, \n\tselector_key CHAR(64) NOT NULL, \n\tversion BIGINT NOT NULL, \n\torganization_id BIGINT NOT NULL, \n\toperator_user_id BIGINT NOT NULL, \n\trequest_json TEXT NOT NULL, \n\treceipt_json TEXT NOT NULL, \n\tCONSTRAINT pk_publication_changes PRIMARY KEY (command_id), \n\tCONSTRAINT fk_publication_changes_selector_key FOREIGN KEY(selector_key) REFERENCES publication_pointers (selector_key), \n\tCONSTRAINT uk_publication_changes_selector_key_version UNIQUE (selector_key, version)\n)ENGINE=InnoDB CHARSET=utf8mb4\n\n",
      "index_sql": [],
      "columns": [
        {
          "name": "command_id",
          "type": "CHAR(36)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "selector_key",
          "type": "CHAR(64)",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "version",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "organization_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "operator_user_id",
          "type": "BIGINT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "request_json",
          "type": "TEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        },
        {
          "name": "receipt_json",
          "type": "TEXT",
          "nullable": false,
          "collation": null,
          "default": null,
          "computed": null,
          "persisted": null
        }
      ],
      "primary_key": [
        "command_id"
      ],
      "indexes": [
        {
          "name": "uk_publication_changes_selector_key_version",
          "columns": [
            "selector_key",
            "version"
          ],
          "unique": true
        }
      ],
      "foreign_keys": [
        {
          "name": "fk_publication_changes_selector_key",
          "columns": [
            "selector_key"
          ],
          "table": "publication_pointers",
          "referred_columns": [
            "selector_key"
          ],
          "ondelete": null
        }
      ],
      "checks": []
    }
  ]
}
"""
if hashlib.sha256(_SOURCE_CONTRACT.encode()).hexdigest() != CONTRACT_SHA256:
    raise RuntimeError("fixed_0040_contract_changed")
CONTRACT = {t["name"]: t for t in json.loads(_SOURCE_CONTRACT)["tables"]}
SPECS = {name: (" ".join(c["name"] for c in t["columns"]), " ".join(t["primary_key"]))
         for name, t in CONTRACT.items()}
SPECS["alembic_version"] = ("version_num", "version_num")
LOGICAL_SPECS = {'clarifications': ['id session_id question_seq text can_skip answer skipped answered_by answered_at', 'id'], 'configuration_publication_changes': ['command_id selector_key version organization_id operator_user_id request_json receipt_json', 'command_id'], 'configuration_publication_pointers': ['selector_key selector_json version active_publication_id changed_at', 'selector_key'], 'configuration_publications': ['publication_id selector_key run_id run_version organization_id content_json content_sha256', 'publication_id'], 'evaluation_admission_locks': ['organization_id', 'organization_id'], 'evaluation_capacity_reservations': ['quota_snapshot run_id organization_id budget_day provider_calls daily_limit requested_by reserved_at', 'run_id'], 'evaluation_checkpoints': ['run_id version checkpoint_json', 'run_id'], 'evaluation_dispatches': ['run_id invocation_id execution_id kind case_id slot_ordinal candidate_id checkpoint_json', 'run_id invocation_id'], 'evaluation_generation_completions': ['run_id execution_id invocation_id case_id slot_ordinal execution_ordinal candidate_id candidate_json evidence_json raw_output normalized_output', 'run_id execution_id'], 'evaluation_policy_assets': ['kind asset_id version fingerprint definition_json source_ref imported_by created_at', 'kind asset_id version'], 'evaluation_response_receipts': ['run_id invocation_id execution_id claim_version definition_json sha256', 'run_id invocation_id'], 'evaluation_run_policies': ['run_id fingerprint definition_json', 'run_id'], 'evaluation_runs': ['run_id execution_mode organization_id requested_by definition_json progress_json', 'run_id'], 'evaluation_semantic_completions': ['run_id execution_id invocation_id candidate_id execution_ordinal evidence_json result_json raw_output normalized_output', 'run_id execution_id'], 'evaluation_slot_claims': ['run_id case_id slot_ordinal version checkpoint_json', 'run_id case_id slot_ordinal'], 'evaluation_suites': ['suite_id suite_version fingerprint definition_json command_id organization_id operator_user_id receipt_json receipt_sha256 source_ref imported_by contracts_json contracts_sha256', 'suite_id suite_version'], 'evidence_sets': ['id session_id fingerprint schema_version items frozen_at', 'id'], 'execution_configurations': ['session_id evidence_set_id evidence_fingerprint publication_id publication_sha256 pointer_version selector_query', 'session_id'], 'execution_jobs': ['id run_id session_id status available_at lease_until fence_token attempt answer skipped question_id', 'id'], 'execution_leases': ['thread_id fence expires_at', 'thread_id'], 'external_requests': ['request_id session_id', 'request_id'], 'idempotency_requests': ['scope_hash key request_hash response created_at', 'scope_hash key'], 'interpretation_artifacts': ['id session_id run_id payload created_at', 'id'], 'interpretation_runs': ['id session_id session_version status checkpoint_ref', 'id'], 'interpretation_sessions': ['id org_id owner_subject_id testee_id assessment_ids goal status version active_run_id current_question_id evidence_set_id workflow_version failure_code created_at updated_at created_at_utc updated_at_utc', 'id'], 'model_calls': ['run_id invocation_id fence_token status request_json response_json failure_code created_at created_at_utc', 'run_id'], 'organization_quota_commands': ['organization_id command_id operator_user_id request_json receipt_json receipt_sha256', 'organization_id command_id'], 'organization_quota_pointers': ['organization_id revision', 'organization_id'], 'organization_quota_versions': ['organization_id revision definition_json definition_sha256 operator_user_id reason created_at', 'organization_id revision'], 'participant_admission_locks': ['organization_id', 'organization_id'], 'participant_capacity_reservations': ['quota_snapshot run_id session_id organization_id subject_id assessment_ids budget_day reserved_at active acquired_at released_at', 'run_id'], 'participant_retries': ['organization_id command_id session_id request_id source_run_id run_id operator_user_id expected_version reason accepted_unknown_risk source_failure_code frozen_request_json receipt created_at', 'organization_id command_id'], 'profile_assets': ['profile_id version fingerprint definition_json source_ref imported_by created_at', 'profile_id version'], 'profile_registrations': ['command_id organization_id operator_user_id profile_id profile_version receipt_json receipt_sha256', 'command_id'], 'prompt_assets': ['template_id version fingerprint package_sha256 package_json source_ref imported_by created_at', 'template_id version'], 'prompt_draft_freezes': ['command_id draft_id organization_id operator_user_id receipt_json receipt_sha256', 'command_id'], 'prompt_draft_revisions': ['command_id draft_id revision organization_id operator_user_id request_json snapshot_json snapshot_sha256', 'command_id'], 'prompt_drafts': ['draft_id organization_id revision', 'draft_id'], 'result_outbox': ['event_id session_id version payload delivered mq_owned attempts created_at delivered_at available_at', 'event_id'], 'route_assets': ['route revision fingerprint definition_json source_ref imported_by created_at', 'route revision'], 'runtime_milestones': ['session_id dedupe_key run_id kind invocation_id attempt occurred_at expires_at', 'session_id dedupe_key'], 'schema_assets': ['schema_id version fingerprint definition_json source_ref imported_by created_at', 'schema_id version'], 'semantic_draft_commands': ['organization_id command_id operator_user_id draft_id request_json receipt_json receipt_sha256', 'organization_id command_id'], 'semantic_draft_heads': ['organization_id draft_id revision', 'organization_id draft_id'], 'semantic_draft_versions': ['organization_id draft_id revision snapshot_json snapshot_sha256', 'organization_id draft_id revision'], 'semantic_prompt_assets': ['organization_id asset_id version fingerprint markdown source_ref imported_by created_at', 'organization_id asset_id version'], 'solution_commands': ['command_id solution_id organization_id operator_user_id request_json receipt_json receipt_sha256', 'command_id'], 'solution_revisions': ['solution_id organization_id revision draft_id state_json state_sha256', 'solution_id'], 'ai_messaging_outbox': ('producer destination message_id body_sha256 body wire wire_sha256 topic kind organization_id aggregate_key aggregate_sequence ordered requires_receipt stage attempts available_at created_at published_at confirmed_at error_code', 'producer destination message_id'), 'ai_messaging_inbox': ('producer message_id destination body_sha256 body wire_sha256 kind aggregate_key reservation_token decision receipt_id received_at', 'producer message_id'), 'ai_messaging_quarantine': ('wire_sha256 wire code logical_producer logical_message_id logical_body_sha256 attempts first_seen_at last_seen_at', 'wire_sha256'), 'ai_messaging_evaluation_sequences': ('run_id sequence version', 'run_id'), 'ai_messaging_observations': ('kind recorded_count recording_since last_observed_at', 'kind')}
RENAMES = {
    "clarifications": "interpretation_clarifications", "evidence_sets": "interpretation_evidence_sets",
    "idempotency_requests": "interpretation_idempotency_requests", "result_outbox": "interpretation_result_outbox",
    "model_calls": "execution_model_calls", "participant_retries": "execution_participant_retries",
    "prompt_draft_freezes": "governance_prompt_draft_freezes", "profile_registrations": "governance_profile_registrations",
    "solution_revisions": "governance_solutions", "solution_commands": "governance_solution_commands",
    "semantic_draft_commands": "governance_semantic_draft_commands",
    "configuration_publications": "publication_records", "configuration_publication_pointers": "publication_pointers",
    "configuration_publication_changes": "publication_changes",
    "evaluation_admission_locks": "quota_evaluation_admission_locks",
    "evaluation_capacity_reservations": "quota_evaluation_capacity_reservations",
    "participant_admission_locks": "quota_participant_admission_locks",
    "participant_capacity_reservations": "quota_participant_capacity_reservations",
    "organization_quota_versions": "quota_organization_versions", "organization_quota_pointers": "quota_organization_pointers",
    "organization_quota_commands": "quota_organization_commands", "ai_messaging_outbox": "messaging_outbox",
    "ai_messaging_inbox": "messaging_inbox", "ai_messaging_quarantine": "messaging_quarantine",
    "ai_messaging_evaluation_sequences": "messaging_evaluation_sequences", "ai_messaging_observations": "messaging_observations",
    "runtime_milestones": "operations_runtime_milestones",
}
ASSETS = {
    "profile": ("profile_assets", "profile_id", "definition_json"),
    "prompt": ("prompt_assets", "template_id", "package_json"),
    "route": ("route_assets", "route", "definition_json"),
    "schema": ("schema_assets", "schema_id", "definition_json"),
    "execution_policy": ("evaluation_policy_assets", "asset_id", "definition_json"),
    "gate_policy": ("evaluation_policy_assets", "asset_id", "definition_json"),
    "semantic_prompt": ("semantic_prompt_assets", "asset_id", "markdown"),
}
MERGED = {"governance_asset_versions", "governance_draft_heads", "governance_draft_versions",
          "evaluation_runs", "evaluation_completions", "interpretation_sessions"}


class Rejected(Exception):
    """Fixed categories only; raw identifiers and bodies are never diagnostics."""


def reject(category):
    raise Rejected(category) from None


def _type(value):
    value = re.sub(r"\s+COLLATE\s+\w+", "", value, flags=re.I).lower().strip()
    value = re.sub(r"\s+character\s+set\s+(?:ascii|utf8mb4)\b", "", value)
    value = re.sub(r"\binteger\b", "int", value)
    return "tinyint(1)" if value in ("bool", "boolean") else value


def validate_schema(table, columns, indexes):
    """Validate fixed physical shape, then caller freezes all actual metadata/DDL.

    Inherited historical character collations are not replaced with a guessed
    template default. Their exact current values, defaults, generated expressions,
    full indexes and DDL must match the independently approved bound on each pass.
    """
    if table == "alembic_version":
        if len(columns) != 1 or columns[0][0] != "version_num" or columns[0][1] not in ("varchar(32)", "varchar(64)") or columns[0][2] != "NO":
            reject("unsupported_0040_system_head_schema")
        if indexes != [("PRIMARY",1,"version_num",0,"A",None,"BTREE","YES",None)]:
            reject("unsupported_0040_system_head_indexes")
        return
    expected = CONTRACT.get(table)
    if expected is None or set(c[0] for c in columns) != set(c["name"] for c in expected["columns"]):
        reject("unsupported_0040_physical_schema")
    actual = {c[0]: c for c in columns}
    for c in expected["columns"]:
        a = actual[c["name"]]
        if _type(c["type"]) != a[1] or a[2] != ("YES" if c["nullable"] else "NO"):
            reject("unsupported_0040_column_type_or_nullability")
        source_default=c["default"]
        actual_default=a[3]
        if source_default is not None and source_default.startswith("CURRENT_TIMESTAMP"):
            if not isinstance(actual_default,str) or actual_default.upper()!=source_default:reject("unsupported_0040_column_default")
        elif actual_default!=source_default:reject("unsupported_0040_column_default")
        extra = a[4].upper()
        auto = any(re.search(r"\b"+re.escape(c["name"])+r"\b.*AUTO_INCREMENT",line,re.I) for line in expected["create_sql"].splitlines())
        if ("AUTO_INCREMENT" in extra)!=auto:reject("unsupported_0040_column_generation")
        if c["computed"] is None:
            if "GENERATED" in extra and "DEFAULT_GENERATED" not in extra:
                reject("unexpected_0040_generated_column")
        elif ("STORED GENERATED" if c["persisted"] else "VIRTUAL GENERATED") not in extra:
            reject("missing_0040_generated_column")
        if c["collation"] is not None and a[5] != c["collation"]:
            reject("unsupported_0040_explicit_collation")
        if c["collation"] is None and a[5] is not None and not re.fullmatch(r"(?:utf8mb4|ascii)_[a-z0-9_]+", a[5]):
            reject("unsupported_0040_inherited_collation")
    by_name = {}
    for row in indexes:
        by_name.setdefault(row[0], []).append(row)
    wanted = {"PRIMARY": (expected["primary_key"], True)}
    wanted.update({i["name"]: (i["columns"], i["unique"]) for i in expected["indexes"]})
    if set(by_name) != set(wanted):
        reject("unsupported_0040_complete_indexes")
    for name, (keys, unique) in wanted.items():
        rows = sorted(by_name[name], key=lambda x: x[1])
        if ([x[2] for x in rows] != keys or any(x[1] != i or x[3] != (0 if unique else 1) or x[4] != "A" or x[5] is not None or x[6] != "BTREE" or x[7] != "YES" or x[8] is not None for i,x in enumerate(rows,1))):
            reject("unsupported_0040_index_shape")



def _expression(sql):
    """Parse the fixed CASE/boolean contract, preserving precedence and literals.

    MySQL adds identifier quoting, charset literal prefixes and redundant
    parentheses. Token stripping without a precedence-preserving parse would
    wrongly accept changed AND/OR meaning, so only this closed grammar is used.
    """
    if not isinstance(sql, str):reject("unsupported_0040_expression")
    token = re.compile(r"\s*(?:(_(?:utf8mb4|ascii))(?=\\?')|(`[^`]+`)|(\\'[^'\\]*\\'|'[^']*')|([0-9]+)|([A-Za-z_][A-Za-z_0-9]*)|(<=|>=|<>|!=|=|<|>|\(|\)|,))",re.I)
    items=[];at=0
    while at<len(sql):
        if not sql[at:].strip():break
        hit=token.match(sql,at)
        if hit is None:reject("unsupported_0040_expression")
        at=hit.end()
        if hit[1]:continue
        if hit[2]:items.append(("id",hit[2][1:-1].lower()))
        elif hit[3]:items.append(("literal",hit[3][2:-2] if hit[3].startswith("\\'") else hit[3][1:-1]))
        elif hit[4]:items.append(("number",str(int(hit[4]))))
        else:items.append(("word",(hit[5] or hit[6]).lower()))
    cursor=0
    def peek():return items[cursor][1] if cursor<len(items) else None
    def take(wanted=None):
        nonlocal cursor
        if cursor>=len(items) or (wanted is not None and peek()!=wanted):reject("unsupported_0040_expression")
        value=items[cursor];cursor+=1;return value
    def atom():
        if peek()=="(":
            take("(");value=expr();take(")");return value
        if peek()=="case":
            take("case");take("when");condition=expr();take("then");yes=expr();take("else");no=expr();take("end")
            return ("case",condition,yes,no)
        value=take()
        if value[0] in ("literal","number"):return value
        if value[1]=="null":return ("null",)
        if peek()=="(":
            if value[1]!="char_length":reject("unsupported_0040_expression")
            take("(");arg=expr();take(")");return ("call",value[1],arg)
        return ("id",value[1])
    def expr(minimum=0):
        left=atom()
        while True:
            op=peek();level=1 if op=="or" else 2 if op=="and" else 3 if op in ("=","!=","<>","<",">","<=",">=","in","not","is") else -1
            if level<minimum:break
            take()
            if op in ("in","not"):
                if op=="not":take("in")
                take("(");values=[expr(4)]
                while peek()==",":take(",");values.append(expr(4))
                take(")");left=("not in" if op=="not" else "in",left,tuple(values))
            elif op=="is":
                neg=peek()=="not"
                if neg:take("not")
                take("null");left=("is not null" if neg else "is null",left)
            else:left=("<>" if op=="!=" else op,left,expr(level+1))
        return left
    value=expr()
    if cursor!=len(items):reject("unsupported_0040_expression")
    return value



def _check_value(node,row):
    op=node[0]
    if op=="id":return row[node[1]]
    if op=="literal":return node[1].encode()
    if op=="number":return int(node[1])
    if op=="null":return None
    if op=="call":
        raw=_check_value(node[2],row)
        if raw is None:return None
        try:return len(raw.decode("utf-8",errors="strict"))
        except Exception:reject("invalid_0040_check_text")
    left=_check_value(node[1],row)
    if op in ("is null","is not null"):return (left is None)==(op=="is null")
    if op in ("in","not in"):
        values=[_check_value(v,row) for v in node[2]]
        result=None if left is None else True if left in values else None if None in values else False
        return None if result is None else not result if op=="not in" else result
    right=_check_value(node[2],row)
    if op=="and":return False if left is False or right is False else None if left is None or right is None else True
    if op=="or":return True if left is True or right is True else None if left is None or right is None else False
    if left is None or right is None:return None
    if type(left) is int and isinstance(right,bytes):right=_number(right)
    if type(right) is int and isinstance(left,bytes):left=_number(left)
    return {"=":lambda:left==right,"<>":lambda:left!=right,"<":lambda:left<right,">":lambda:left>right,
            "<=":lambda:left<=right,">=":lambda:left>=right}[op]()

def validate_constraints(table, generated, foreign_keys, checks, database_name):
    """The complete actual constraints are frozen independently, too."""
    if table=="alembic_version":
        if foreign_keys or checks or generated!=[("version_num","")]:reject("unsupported_0040_system_constraints")
        return
    expected=CONTRACT[table]
    gen={c["name"]:c["computed"] for c in expected["columns"]}
    if len(generated)!=len(gen) or {x[0] for x in generated}!=set(gen):reject("unsupported_0040_generated_metadata")
    for name,sql in generated:
        if gen[name] is None:
            if sql!="":reject("unexpected_0040_generated_expression")
        elif _expression(sql)!=_expression(gen[name]):reject("0040_generated_expression_conflict")
    wanted=[]
    for f in expected["foreign_keys"]:
        for i,(child,parent) in enumerate(zip(f["columns"],f["referred_columns"]),1):
            wanted.append((f["name"],i,child,database_name,f["table"],parent,"NO ACTION",f["ondelete"] or "NO ACTION"))
    if sorted(foreign_keys)!=sorted(wanted):reject("unsupported_0040_complete_foreign_keys")
    if len(checks)!=len(expected["checks"]):reject("unsupported_0040_complete_checks")
    wanted={c["name"]:_expression(c["sql"]) for c in expected["checks"]}
    if {c[0] for c in checks}!=set(wanted):reject("unsupported_0040_complete_checks")
    for name,sql,enforced in checks:
        if enforced!="YES" or _expression(sql)!=wanted[name]:reject("0040_check_constraint_conflict")

def _frame(h, raw):
    h.update(b"\x00" + struct.pack(">Q", 0) if raw is None else b"\x01" + struct.pack(">Q", len(raw)) + raw)


def _number(raw):
    if not isinstance(raw, bytes) or not re.fullmatch(rb"0|[1-9][0-9]*", raw):
        reject("invalid_0040_numeric_fact")
    return int(raw)


def _generated(row, expected):
    if any(row[key] != value for key, value in expected.items()):
        reject("0040_generated_identity_conflict")


def project(dataset, logical_specs, schemas):
    """Only a complete raw physical collection may become logical business rows.

    Return separate body-free source lineage. No ID, policy, Run, timestamp,
    message receipt or missing legacy fact is generated or inferred.
    """
    if set(dataset) != set(SPECS) or set(schemas) != set(SPECS):
        reject("complete_0040_physical_scan_required")
    if dataset["alembic_version"] != [{"version_num": HEAD.encode()}]:
        reject("0040_system_head_source_conflict")
    for name, rows in dataset.items():
        cols, keys = SPECS[name][0].split(), SPECS[name][1].split()
        seen = set()
        nonnull = {c["name"] for c in CONTRACT[name]["columns"] if not c["nullable"]} if name in CONTRACT else {"version_num"}
        for row in rows:
            if set(row) != set(cols) or any(v is not None and not isinstance(v, bytes) for v in row.values()) or any(row[c] is None for c in nonnull):
                reject("0040_raw_row_shape_or_null_conflict")
            key = tuple(row[c] for c in keys)
            if key in seen:
                reject("0040_duplicate_physical_identity")
            seen.add(key)
    # Inspect every declared FK against the full raw parent collection, before
    # any kind filter, JOIN, organization predicate or logical compatibility view.
    for name, contract in CONTRACT.items():
        for fk in contract["foreign_keys"]:
            parents = {tuple(r[k] for k in fk["referred_columns"]) for r in dataset[fk["table"]]}
            for row in dataset[name]:
                ref = tuple(row[k] for k in fk["columns"])
                if all(v is not None for v in ref) and ref not in parents:
                    reject("0040_global_physical_orphan_or_owner_conflict")
    logical = {name: [] for name in logical_specs}
    lineage = {name: [] for name in logical_specs}
    def add(name, values, physical, original):
        if name not in logical_specs:
            reject("unsupported_0040_logical_destination")
        cols = logical_specs[name][0].split()
        if any(k not in values for k in cols):
            reject("0040_original_field_not_preserved")
        logical[name].append({k: values[k] for k in cols})
        h = hashlib.sha256(); _frame(h, schemas[physical]["columns_sha256"].encode())
        for c in schemas[physical]["columns"]:
            _frame(h, original[c[0]])
        lineage[name].append({"physical_table": physical,
            "physical_pk": tuple(original[k] for k in SPECS[physical][1].split()),
            "physical_row_sha256": h.hexdigest()})
    for old in logical_specs:
        physical = RENAMES.get(old, old)
        if physical in MERGED or physical not in dataset:
            continue
        for row in dataset[physical]:
            add(old, row, physical, row)
    for row in dataset["governance_asset_versions"]:
        try:
            kind = row["asset_kind"].decode("ascii")
        except Exception:
            reject("unknown_0040_asset_kind")
        if kind not in ASSETS:
            reject("unknown_0040_asset_kind")
        old, identity, body = ASSETS[kind]
        owner = _number(row["owner_organization_id"])
        if kind != "semantic_prompt" and owner != 0:
            reject("0040_asset_owner_conflict")
        fmt = b"prompt_package_json" if kind == "prompt" else b"semantic_markdown" if kind == "semantic_prompt" else b"definition_json"
        if row["body_format"] != fmt or ((row["package_sha256"] is None) == (kind == "prompt")):
            reject("0040_asset_body_shape_conflict")
        _generated(row, {"native_id_key": row["asset_id"] if kind in ("profile", "prompt", "route", "schema") else None,
            "scoped_id_key": row["asset_id"] if kind in ("execution_policy", "gate_policy", "semantic_prompt") else None,
            "profile_id_key": row["asset_id"] if kind == "profile" else None, "catalog_version_key": row["version"]})
        values = {**row, identity: row["asset_id"], body: row["body_bytes"], "organization_id": row["owner_organization_id"], "revision": row["version"]}
        if kind in ("execution_policy", "gate_policy"):
            values["kind"] = b"execution" if kind == "execution_policy" else b"gate"
        add(old, values, "governance_asset_versions", row)
    heads = {}
    for row in dataset["governance_draft_heads"]:
        kind = row["draft_kind"]
        if kind not in (b"prompt", b"semantic"):
            reject("unknown_0040_draft_kind")
        _number(row["organization_id"]); _number(row["revision"])
        _generated(row, {"prompt_draft_id_key": row["draft_id"] if kind == b"prompt" else None,
                         "semantic_draft_id_key": row["draft_id"] if kind == b"semantic" else None})
        heads[row["draft_row_id"]] = row
        add("prompt_drafts" if kind == b"prompt" else "semantic_draft_heads", row, "governance_draft_heads", row)
    for row in dataset["governance_draft_versions"]:
        head = heads.get(row["draft_row_id"]); kind = row["draft_kind"]
        if kind not in (b"prompt", b"semantic"):
            reject("unknown_0040_draft_kind")
        if head is None or any(row[k] != head[k] for k in ("draft_kind", "organization_id")) or not 0 < _number(row["revision"]) <= _number(head["revision"]):
            reject("0040_draft_parent_or_revision_conflict")
        audit = [row[k] for k in ("command_id", "operator_user_id", "request_bytes")]
        if (kind == b"prompt" and any(v is None for v in audit)) or (kind == b"semantic" and any(v is not None for v in audit)):
            reject("0040_draft_audit_null_conflict")
        _generated(row, {"prompt_command_id_key": row["command_id"] if kind == b"prompt" else None})
        if hashlib.sha256(row["snapshot_bytes"]).hexdigest().encode() != row["snapshot_sha256"]:
            reject("0040_original_draft_bytes_digest_conflict")
        values = {**row, "draft_id": head["draft_id"], "snapshot_json": row["snapshot_bytes"], "request_json": row["request_bytes"]}
        add("prompt_draft_revisions" if kind == b"prompt" else "semantic_draft_versions", values, "governance_draft_versions", row)
    for row in dataset["interpretation_sessions"]:
        add("interpretation_sessions", row, "interpretation_sessions", row)
        if row["request_id"] is not None:
            if not row["request_id"]:
                reject("0040_empty_external_request_identity")
            add("external_requests", {"request_id": row["request_id"], "session_id": row["id"]}, "interpretation_sessions", row)
    for row in dataset["evaluation_runs"]:
        add("evaluation_runs", row, "evaluation_runs", row)
        fp, body = row["frozen_execution_policy_fingerprint"], row["frozen_execution_policy_json"]
        if (fp is None) != (body is None):
            reject("0040_original_policy_single_null_conflict")
        if fp is not None:
            add("evaluation_run_policies", {"run_id": row["run_id"], "fingerprint": fp, "definition_json": body}, "evaluation_runs", row)
    evaluation_ids = {r["run_id"] for r in dataset["evaluation_runs"]}
    for row in dataset["evaluation_completions"]:
        kind = row["kind"]
        if kind not in (b"generation", b"semantic"):
            reject("unknown_0040_completion_kind")
        if (kind == b"generation" and (row["case_id"] is None or row["slot_ordinal"] is None or row["result_json"] is not None)) or (kind == b"semantic" and (row["candidate_id"] is None or any(row[k] is not None for k in ("case_id", "slot_ordinal", "candidate_json")))):
            reject("0040_completion_kind_null_conflict")
        _generated(row, {"gen_candidate_key": row["candidate_id"] if kind == b"generation" else None,
            "gen_case_key": row["case_id"] if kind == b"generation" else None,
            "gen_slot_key": row["slot_ordinal"] if kind == b"generation" else None,
            "sem_candidate_key": row["candidate_id"] if kind == b"semantic" else None})
        if row["run_id"] not in evaluation_ids:
            reject("0040_global_completion_run_orphan")
        add("evaluation_generation_completions" if kind == b"generation" else "evaluation_semantic_completions", row, "evaluation_completions", row)
    for name, contract in CONTRACT.items():
        predicates=[_expression(c["sql"]) for c in contract["checks"]]
        for row in dataset[name]:
            if any(_check_value(p,row) is False for p in predicates):
                reject("0040_source_row_check_constraint_conflict")
    for name, rows in logical.items():
        keys = logical_specs[name][1].split(); seen = set()
        for row in rows:
            key = tuple(row[k] for k in keys)
            if any(k is None for k in key) or key in seen:
                reject("0040_logical_identity_collision")
            seen.add(key)
    expected_keys={(name,tuple(row[k] for k in SPECS[name][1].split()))
                   for name,rows in dataset.items() if name!="alembic_version" for row in rows}
    projected_keys={(ref["physical_table"],ref["physical_pk"]) for refs in lineage.values() for ref in refs}
    if expected_keys!=projected_keys:
        reject("0040_physical_row_projection_coverage_gap")
    return logical, lineage
