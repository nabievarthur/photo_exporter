package main

import (
	"bufio"
	"database/sql"
	"fmt"
	_ "github.com/sijms/go-ora/v2"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	time.Sleep(1 * time.Second)
	fmt.Println("=== Photo Exporter запущен ===")

	config := GetConfig()

	// Пробуем разные варианты подключения
	connectionVariants := []string{
		config.GetDSN(),
	}

	// Добавляем альтернативные варианты если есть и сервис и SID
	if config.DBService != "" && config.DBSID != "" {
		escapedUser := url.QueryEscape(config.DBUser)
		escapedPassword := url.QueryEscape(config.DBPassword)

		connectionVariants = append(connectionVariants,
			fmt.Sprintf("oracle://%s:%s@%s:%s?sid=%s",
				escapedUser, escapedPassword, config.DBHost, config.DBPort, config.DBSID))
	}

	var db *sql.DB
	var err error

	for i, dsn := range connectionVariants {
		fmt.Printf("\nПопытка подключения %d:\n", i+1)
		fmt.Printf("DSN: %s\n", maskPassword(dsn))

		db, err = sql.Open("oracle", dsn)
		if err != nil {
			fmt.Printf("❌ Ошибка открытия БД: %v\n", err)
			continue
		}

		// Устанавливаем таймаут
		db.SetConnMaxLifetime(time.Minute * 3)
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)

		fmt.Println("Проверяем подключение...")
		err = db.Ping()
		if err != nil {
			fmt.Printf("❌ Ошибка ping: %v\n", err)
			db.Close()
			continue
		}

		fmt.Println("✅ Подключение успешно!")
		break
	}

	if err != nil {
		log.Printf("❌ Все попытки подключения failed: %v", err)
		fmt.Println("\nВозможные решения:")
		fmt.Println("1. Проверьте правильность SERVICE_NAME или SID")
		fmt.Println("2. Убедитесь что сервер доступен")
		fmt.Println("3. Проверьте firewall и сетевые настройки")
		fmt.Println("4. Попробуйте использовать IP вместо hostname")
		waitForInput()
		return
	}
	defer db.Close()

	// Проверяем доступ к таблице
	fmt.Println("\nПроверяем доступ к таблице T019_FOTO...")
	var tableCount int
	err = db.QueryRow(`
		SELECT COUNT(*) 
		FROM T019_FOTO FT
		INNER JOIN T019 ROZ ON ROZ.NSYST = FT.NSYST
		WHERE FT.IBD_ARX = 1 
		  AND ROZ.KODRAI1 LIKE '%БАШК%'
		  AND (UPPER(FT.FILEFORMAT) LIKE '%JPEG%' 
			OR UPPER(FT.FILEFORMAT) LIKE '%JPG%' 
			OR UPPER(FT.TEXT) LIKE '%.JPG%' 
			OR UPPER(FT.TEXT) LIKE '%.JPEG%')
	`).Scan(&tableCount)

	if err != nil {
		log.Printf("❌ Ошибка доступа к таблице: %v", err)
		waitForInput()
		return
	}

	fmt.Printf("✅ Найдено записей для экспорта (с фильтром БАШК): %d\n", tableCount)

	if tableCount == 0 {
		fmt.Println("⚠ Нет фотографий для экспорта (IBD_ARX = 1 и KODRAI1 LIKE '%БАШК%')")
		waitForInput()
		return
	}

	// Получаем фотографии
	err = getPhotos(db)
	if err != nil {
		log.Printf("Ошибка получения фотографий: %v", err)
		waitForInput()
		return
	}

	fmt.Println("✓ Программа завершена успешно")
	waitForInput()
}

// Функция для маскировки пароля в логах
func maskPassword(dsn string) string {
	return dsn
}

func getPhotos(db *sql.DB) error {
	fmt.Println("Выполняем SQL запрос...")

	query := `SELECT FT.NSYST, FT.NPSYST, FT.DT_REG, FT.SH_POLZ, FT.TEXT, FT.SIGN, FT.IMG, 
       FT.DOSTUP, FT.IBD_ARX, FT.FILEFORMAT,
       ROZ.FAM, ROZ.IMJ, ROZ.OTCH, ROZ.Y_ROJD, ROZ.M_ROJD, ROZ.D_ROJD
FROM T019_FOTO FT
INNER JOIN T019 ROZ ON ROZ.NSYST = FT.NSYST
WHERE FT.IBD_ARX = 1 
  AND ROZ.KODRAI1 LIKE '%БАШК%'
  AND (UPPER(FT.FILEFORMAT) LIKE '%JPEG%' 
    OR UPPER(FT.FILEFORMAT) LIKE '%JPG%' 
    OR UPPER(FT.TEXT) LIKE '%.JPG%' 
    OR UPPER(FT.TEXT) LIKE '%.JPEG%')`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса: %v", err)
	}
	defer rows.Close()

	outputDir := "photos"
	fmt.Printf("Создаем папку для сохранения: %s\n", outputDir)
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("ошибка создания папки: %v", err)
	}

	var count int
	fmt.Println("Обрабатываем результаты...")

	for rows.Next() {
		var photo Photo
		err := rows.Scan(
			&photo.NSYST,
			&photo.NPSYST,
			&photo.DT_REG,
			&photo.SH_POLZ,
			&photo.TEXT,
			&photo.SIGN,
			&photo.IMG,
			&photo.DOSTUP,
			&photo.IBD_ARX,
			&photo.FILEFORMAT,
			&photo.FAM,
			&photo.IMJ,
			&photo.OTCH,
			&photo.Y_ROJD,
			&photo.M_ROJD,
			&photo.D_ROJD,
		)
		if err != nil {
			log.Printf("Ошибка чтения строки: %v", err)
			continue
		}

		err = savePhoto(photo, outputDir)
		if err != nil {
			log.Printf("Ошибка сохранения фото NSYST=%d: %v", photo.NSYST, err)
			continue
		}

		count++
		fmt.Printf("✓ Сохранена фотография: %s (NSYST=%d)\n", getFileName(photo), photo.NSYST)
	}

	if count == 0 {
		fmt.Println("⚠ Фотографии не найдены по заданным критериям")
	} else {
		fmt.Printf("✓ Успешно сохранено фотографий: %d\n", count)
	}

	return nil
}

func getFileName(photo Photo) string {
	// Формируем имя файла из ФИО и даты рождения
	var nameParts []string

	// Добавляем фамилию
	if photo.FAM.Valid && photo.FAM.String != "" {
		nameParts = append(nameParts, photo.FAM.String)
	}

	// Добавляем имя
	if photo.IMJ.Valid && photo.IMJ.String != "" {
		nameParts = append(nameParts, photo.IMJ.String)
	}

	// Добавляем отчество
	if photo.OTCH.Valid && photo.OTCH.String != "" {
		nameParts = append(nameParts, photo.OTCH.String)
	}

	// Добавляем дату рождения если есть все компоненты
	if photo.Y_ROJD.Valid && photo.M_ROJD.Valid && photo.D_ROJD.Valid {
		year := photo.Y_ROJD.Int64
		month := photo.M_ROJD.Int64
		day := photo.D_ROJD.Int64
		if year > 0 && month > 0 && day > 0 {
			nameParts = append(nameParts, fmt.Sprintf("%02d.%02d.%04d", day, month, year))
		}
	}

	// Если есть какие-то данные для имени
	if len(nameParts) > 0 {
		fileName := strings.Join(nameParts, "_")
		fileName = sanitizeFileName(fileName)
		fileName += ".jpg" // Добавляем расширение
		return fileName
	}

	// Если нет ФИО, используем оригинальное имя или генерируем по NSYST
	if photo.TEXT.Valid && photo.TEXT.String != "" {
		fileName := filepath.Base(photo.TEXT.String)
		fileName = sanitizeFileName(fileName)
		return fileName
	}

	return fmt.Sprintf("photo_%d_%d.jpg", photo.NSYST, photo.NPSYST)
}

// Функция для очистки имени файла от недопустимых символов
func sanitizeFileName(fileName string) string {
	// Заменяем недопустимые символы в Windows
	invalidChars := []string{"\\", "/", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalidChars {
		fileName = strings.ReplaceAll(fileName, char, "_")
	}

	// Убираем начальные и конечные пробелы
	fileName = strings.TrimSpace(fileName)

	// Заменяем множественные пробелы и подчеркивания
	fileName = strings.ReplaceAll(fileName, "  ", " ")
	fileName = strings.ReplaceAll(fileName, "__", "_")

	return fileName
}

func savePhoto(photo Photo, outputDir string) error {
	if len(photo.IMG) == 0 {
		return fmt.Errorf("изображение пустое")
	}

	fileName := getFileName(photo)
	filePath := filepath.Join(outputDir, fileName)

	return os.WriteFile(filePath, photo.IMG, 0644)
}

func waitForInput() {
	fmt.Println("\nНажмите Enter для выхода...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
