package postgresql

import (
	"fmt"
	"github.com/TMS360/backend-pkg/config"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Client struct {
}

func NewClient(cfg config.PostgresSQLConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port, cfg.SSLMode, cfg.TimeZone)

	fmt.Println("dsn: ", dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	//connection, err := db.DB()
	//if err != nil {
	//	log.Fatalf("failed to connect database: %v", err)
	//}

	//connection.SetMaxIdleConns(idleConnection)
	//connection.SetMaxOpenConns(maxConnection)
	//connection.SetConnMaxLifetime(time.Second * time.Duration(maxLifeTimeConnection))

	return db, nil
}
