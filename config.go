package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
)

type Config struct {
	DBUser     string
	DBPassword string
	DBHost     string
	DBPort     string
	DBService  string
	DBSID      string
}

type Photo struct {
	NSYST      int64
	NPSYST     int64
	DT_REG     sql.NullTime
	SH_POLZ    sql.NullInt64
	TEXT       sql.NullString
	SIGN       sql.NullString
	IMG        []byte
	DOSTUP     sql.NullString
	IBD_ARX    sql.NullInt64
	FILEFORMAT sql.NullString
	// Новые поля из T019
	FAM    sql.NullString
	IMJ    sql.NullString
	OTCH   sql.NullString
	Y_ROJD sql.NullInt64
	M_ROJD sql.NullInt64
	D_ROJD sql.NullInt64
}

func GetConfig() Config {
	config := Config{
		DBUser:     getEnv("DB_USER", ""),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBHost:     getEnv("DB_HOST", ""),
		DBPort:     getEnv("DB_PORT", "1521"),
		DBService:  getEnv("DB_SERVICE", "ORCL"),
		DBSID:      getEnv("DB_SID", ""),
	}

	if config.DBUser == "" || config.DBPassword == "" || config.DBHost == "" {
		fmt.Println("❌ Ошибка: Не заданы параметры подключения к БД")
		fmt.Println("Установите переменные окружения:")
		fmt.Println("DB_USER=username")
		fmt.Println("DB_PASSWORD=password")
		fmt.Println("DB_HOST=hostname")
		fmt.Println("DB_PORT=1521")
		fmt.Println("DB_SERVICE=service_name ИЛИ DB_SID=sid_name")
		return config
	}

	fmt.Println("✓ Параметры подключения получены")
	return config
}

func (c Config) GetDSN() string {
	escapedUser := url.QueryEscape(c.DBUser)
	escapedPassword := url.QueryEscape(c.DBPassword)

	if c.DBService != "" {
		return fmt.Sprintf("oracle://%s:%s@%s:%s/%s",
			escapedUser, escapedPassword, c.DBHost, c.DBPort, c.DBService)
	} else if c.DBSID != "" {
		return fmt.Sprintf("oracle://%s:%s@%s:%s?sid=%s",
			escapedUser, escapedPassword, c.DBHost, c.DBPort, c.DBSID)
	} else {
		return fmt.Sprintf("oracle://%s:%s@%s:%s",
			escapedUser, escapedPassword, c.DBHost, c.DBPort)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
