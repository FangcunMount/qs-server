package configcontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDBOpsMongoBackupRunsOnMacMiniAndIsFailClosed(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "db-ops.yml"))
	if err != nil {
		t.Fatalf("read db ops workflow: %v", err)
	}
	backupScript, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "dbops", "mongodb-mac-mini-backup.sh"))
	if err != nil {
		t.Fatalf("read Mac mini Mongo backup script: %v", err)
	}
	workflowContent := string(workflow)
	for _, required := range []string{
		`name: MongoDB Backup on Mac mini`,
		`group: qlume`,
		`labels: [self-hosted, macOS, ARM64, ops]`,
		`timeout-minutes: 360`,
		`MONGO_BACKUP_RETENTION_COUNT: '3'`,
		`MONGO_BACKUP_SSH_FINGERPRINT: ${{ secrets.MONGO_BACKUP_SSH_FINGERPRINT }}`,
		`run: bash scripts/dbops/mongodb-mac-mini-backup.sh`,
	} {
		if !strings.Contains(workflowContent, required) {
			t.Errorf("DB ops Mac mini Mongo backup workflow must contain %q", required)
		}
	}

	scriptContent := string(backupScript)
	for _, required := range []string{
		`MONGO_BACKUP_RETENTION_COUNT:-3`,
		`if ! [[ "$BACKUP_DIR" == /*/backups/qs-server/mongodb ]]`,
		`ssh-keyscan -T 10`,
		`grep -Fqx -- "$MONGO_BACKUP_SSH_FINGERPRINT"`,
		`ExitOnForwardFailure=yes`,
		`socketTimeoutMS=0`,
		`trap cleanup EXIT`,
		`--numParallelCollections=4`,
		`--archive="$PARTIAL_FILE"`,
		`validate_archive "$PARTIAL_FILE"`,
		`mv -- "$PARTIAL_FILE" "$FINAL_FILE"`,
		`rm -f -- "$old_backup" "$old_backup.sha256"`,
	} {
		if !strings.Contains(scriptContent, required) {
			t.Errorf("Mac mini Mongo backup script must contain %q", required)
		}
	}
	for _, forbidden := range []string{
		`BACKUP_CONTAINER_NAME="qs-server-mongodb-backup"`,
		`com.fangcunmount.qs-server.operation=mongodb-backup`,
		`mongo:7.0 345m /bin/bash`,
		`for ATTEMPT in 1 2 3`,
		`--archive | gzip`,
	} {
		if strings.Contains(workflowContent, forbidden) || strings.Contains(scriptContent, forbidden) {
			t.Errorf("Mac mini Mongo backup path must not contain %q", forbidden)
		}
	}
	if strings.Contains(scriptContent, `--password=`) {
		t.Error("Mac mini Mongo backup script must not expose the password through process arguments")
	}
}
