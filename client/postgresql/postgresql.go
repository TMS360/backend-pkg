package postgresql

import (
	"fmt"
	"log"

	"github.com/TMS360/backend-pkg/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Client struct {
}

func NewClient(cfg config.PostgresSQLConfig) (*gorm.DB, error) {
	initialDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=postgres port=%s sslmode=%s TimeZone=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.Port, cfg.SSLMode, cfg.TimeZone)

	db, err := gorm.Open(postgres.Open(initialDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to postgres database: %v", err)
	}

	var exists bool
	db.Raw("SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = ?)", cfg.DBName).Scan(&exists)

	if !exists {
		err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", cfg.DBName)).Error
		if err != nil {
			log.Fatalf("failed to create database %s: %v", cfg.DBName, err)
		}
		log.Printf("Database %s created successfully", cfg.DBName)
	} else {
		log.Printf("Database %s already exists", cfg.DBName)
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port, cfg.SSLMode, cfg.TimeZone)

	fmt.Println("dsn: ", dsn)

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	return db, nil
}
