// Package maintenance supports explicit QS maintenance commands.
package maintenance

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/database"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenMySQL opens the small, silent pool used by operator maintenance commands.
// The caller owns the connection and must close it after the command completes.
func OpenMySQL(configured *genericoptions.MySQLOptions, purpose string) (*database.MySQLConnection, error) {
	if configured == nil {
		return nil, fmt.Errorf("qs MySQL options are required")
	}
	connection := database.NewMySQLConnection(&database.MySQLConfig{
		Host: configured.Host, Username: configured.Username, Password: configured.Password, Database: configured.Database,
		MaxIdleConnections: 2, MaxOpenConnections: 10, MaxConnectionLifeTime: configured.MaxConnectionLifeTime,
		LogLevel: configured.LogLevel, Location: configured.Location, SessionTimeZone: configured.SessionTimeZone,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err := connection.Connect(); err != nil {
		return nil, fmt.Errorf("connect QS MySQL for %s", purpose)
	}
	db, ok := connection.GetClient().(*gorm.DB)
	if !ok || db == nil {
		_ = connection.Close()
		return nil, fmt.Errorf("resolve qs MySQL client")
	}
	if _, err := db.DB(); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("resolve qs SQL client: %w", err)
	}
	return connection, nil
}
