package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	generated "github.com/Lirikman/user-subscriptions/db/generated"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
)

// создание маршрутизатора Gin
func setupRouter() *gin.Engine {
	router := gin.Default()
	router.ForwardedByClientIP = true
	// настраиваем доверенные прокси
	proxies := []string{"127.0.0.1", "::1"}
	err := router.SetTrustedProxies(proxies)
	if err != nil {
		log.Fatalf("error while setting up proxy")
	}
	// настройка политики разрешений
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"https://localhost:8080/"}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE"}
	config.AllowCredentials = true
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	router.Use(cors.New(config))
	// подключаем инструмент восстановления сбоев
	router.Use(gin.Recovery())
	// подключаем логгер
	router.Use(gin.Logger())
	// задаём стандартный маршрут '/ping'
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	return router
}

// DTO для запроса создания записи
type CreateSubReqDTO struct {
	UserID      string `json:"user_id" binding:"required"`
	ServiceName string `json:"service_name" binding:"required"`
	Price       int32  `json:"price" binding:"required"`
	StartDate   string `json:"start_date" binding:"required"`
	EndDate     string `json:"end_date"`
}

// DTO для входящего обновления записи
type UpdateSubReqDTO struct {
	UserID      string `json:"user_id" binding:"required"`
	ServiceName string `json:"service_name" binding:"required"`
	Price       int32  `json:"price" binding:"required"`
	StartDate   string `json:"start_date" binding:"required"`
	EndDate     string `json:"end_date"`
}

// DTO для получения общей стоимости подписок
type TotalPriceDTO struct {
	UserID      string `json:"user_id" binding:"required"`
	ServiceName string `json:"service_name" binding:"required"`
	StartDate   string `json:"start_date" binding:"required"`
	EndDate     string `json:"end_date" binding:"required"`
}

// создание записи о подписке
func CreateSubscription(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateSubReqDTO

		// десериализация данных тела запроса
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		// проверяем user_id на корректность
		var userUUID pgtype.UUID
		err := userUUID.Scan(req.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"})
			return
		}
		// проверяем корректность ввода цены
		if req.Price <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subscription price must be positive"})
			return
		}
		// проверяем корректность ввода периода подписки
		var endDate pgtype.Date
		layout := "01-2006" // шаблон формата времени
		parsStrDate, err := time.Parse(layout, req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subscription start date is incorrect (example start date: 07-2025)"})
			return
		}
		if req.EndDate != "" {
			parsEndDate, err := time.Parse(layout, req.EndDate)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "subscription end date is incorrect (example: 05-2026)"})
				return
			}
			if parsStrDate.Compare(parsEndDate) > 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "the subscription period is set incorrectly"})
				return
			}
			endDate = pgtype.Date{Time: parsEndDate, Valid: true}
		}
		strDate := pgtype.Date{Time: parsStrDate, Valid: true}

		// формируем параметры для запроса
		params := generated.CreateSubscriptionParams{
			UserID:      userUUID,
			ServiceName: req.ServiceName,
			Price:       req.Price,
			StartDate:   strDate,
			EndDate:     endDate,
		}

		// выполняем запрос к БД
		subsc, err := db.CreateSubscription(context.Background(), params)
		if err != nil {
			log.Printf("error create user subscription: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		c.JSON(http.StatusCreated, subsc)
	}
}

// получение всех записей о подписках
func ListSubscriptions(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		subsrciptions, err := db.ListSubscriptions(c)
		if err != nil {
			// ошибка сервера
			log.Printf("error getting list of user subscriptions: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		// проверяем, вызвана ли ошибка отсутствием записей в БД
		if len(subsrciptions) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "no subscriptions found"})
			return
		}
		c.JSON(http.StatusOK, subsrciptions)
	}
}

// получение всех записей пользователя по user_id
func GetSubscriptionsFromUserId(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var userUUID pgtype.UUID
		// чтение параметра user_id из запроса
		userIDStr := c.Param("user_id")

		// проверяем user_id на корректность
		err := userUUID.Scan(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"})
			return
		}

		// получем все записи пользователя о подписках
		subsc, err := db.GetSubscriptions(c, userUUID)
		if err != nil {
			// ошибка сервера
			log.Printf("error retrieving user subscriptions: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		// проверяем, вызвана ли ошибка отсутствием записи в БД
		if len(subsc) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "user with this user_id not found"})
			return
		}
		c.JSON(http.StatusOK, subsc)
	}
}

// обновление записи о подписке
func UpdateSubscription(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req UpdateSubReqDTO
		var userUUID pgtype.UUID

		// чтение параметра id из запроса
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "incorrect ID value entered"})
			return
		}
		// проверяем наличие записи в БД
		_, err = db.GetSubscriptionFromID(c, id)
		if err != nil {
			// проверяем, вызвана ли ошибка отсутствием строки в БД
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "no records with this ID were found"})
				return
			}
			// иначе это другая ошибка сервера
			log.Printf("error retrieving a record from the database by ID: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		// десериализация данных тела запроса
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		// проверяем user_id на корректность
		err = userUUID.Scan(req.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"})
			return
		}

		// проверяем корректность ввода цены
		if req.Price <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subscription price must be positive"})
			return
		}

		// проверяем корректность ввода периода подписки
		var endDate pgtype.Date
		layout := "01-2006" // шаблон формата времени
		parsStrDate, err := time.Parse(layout, req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subscription start date is incorrect (example start date: 07-2025)"})
			return
		}
		if req.EndDate != "" {
			parsEndDate, err := time.Parse(layout, req.EndDate)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "subscription end date is incorrect (example: 05-2026)"})
				return
			}
			if parsStrDate.Compare(parsEndDate) > 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "subscription period is set incorrectly"})
				return
			}
			endDate = pgtype.Date{Time: parsEndDate, Valid: true}
		}
		strDate := pgtype.Date{Time: parsStrDate, Valid: true}

		// формируем параметры для запроса
		params := generated.UpdateSubscriptionParams{
			ID:          id,
			UserID:      userUUID,
			ServiceName: req.ServiceName,
			Price:       req.Price,
			StartDate:   strDate,
			EndDate:     endDate,
		}
		// обновляем запись
		res, err := db.UpdateSubscription(c, params)
		if err != nil {
			log.Printf("user subscription update error: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		c.JSON(http.StatusOK, res)
	}
}

// удаление всех подписок пользователя
func DeleteSubscription(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		// чтение параметра id из запроса
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "incorrect ID value entered"})
			return
		}
		// проверяем наличие записи в БД
		_, err = db.GetSubscriptionFromID(c, id)
		if err != nil {
			// проверяем, вызвана ли ошибка отсутствием строки в БД
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "no records with this ID were found"})
				return
			}
			// иначе это другая ошибка сервера
			log.Printf("error retrieving a record from the database by ID: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		// удаляем запись
		err = db.DeleteSubscription(c, id)
		if err != nil {
			log.Printf("error deleting user subscription: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "entry with ID " + idStr + " has been successfully deleted"})
	}
}

// получение общей стоимости подписок за выбранный период
func TotalCostSubscription(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TotalPriceDTO
		var userUUID pgtype.UUID

		// десериализация данных тела запроса
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		// чтение параметра user_id из запроса
		userIDStr := req.UserID

		// проверяем user_id на корректность
		err := userUUID.Scan(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"})
			return
		}

		// проверка наличия записи в БД
		subsc, err := db.GetSubscriptions(c, userUUID)
		if err != nil {
			// ошибка сервера
			log.Printf("error retrieving user subscriptions: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		// проверяем, вызвана ли ошибка отсутствием строки в БД
		if len(subsc) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "user with this user ID will not be found"})
			return
		}

		// проверка наличия сервиса в БД
		svcName, err := db.CheckingServiceName(c, req.ServiceName)
		if err != nil {
			log.Printf("checking service name error: %v\n", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "service name you entered was not found"})
			return
		}

		// проверка корректности ввода периода подписки
		layout := "01-2006" // шаблон формата времени
		parsStrDate, err := time.Parse(layout, req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subscription start date is incorrect (example start date: 07-2025)"})
			return
		}
		parsEndDate, err := time.Parse(layout, req.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subscription end date is incorrect (example end date: 05-2026)"})
			return
		}
		if parsStrDate.Compare(parsEndDate) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the subscription period is set incorrectly"})
			return
		}
		strDate := pgtype.Date{Time: parsStrDate, Valid: true}
		endDate := pgtype.Date{Time: parsEndDate, Valid: true}

		// формируем параметры для запроса
		params := generated.TotalPriceSubscriptionParams{
			UserID:      userUUID,
			ServiceName: svcName,
			StartDate:   strDate,
			EndDate:     endDate,
		}
		res, err := db.TotalPriceSubscription(c, params)
		if err != nil {
			log.Printf("error getting the total cost of subscriptions: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		message := fmt.Sprintf("total price - %v rubles", res)
		c.JSON(http.StatusOK, gin.H{"message": message})
	}
}

func main() {
	// загружаем переменные окружения
	err := godotenv.Load()
	if err := godotenv.Load(); err != nil {
		log.Println("warning: .env file not found, reading system variables")
	}
	host := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	pathMigrations := os.Getenv("PATH_MIGRATIONS")

	// формируем строку подключения к локальной базе данных
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, dbPort, dbName)

	// применение миграций
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("connection error: %v", err)
	}
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("dialect installation error: %v", err)
	}
	if err := goose.Up(db, pathMigrations); err != nil {
		log.Fatalf("error applying migrations: %v", err)
	}
	log.Println("migrations have been successfully applied")

	// Инициализация пула соединений
	conn, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Error connecting to the database: %v\n", err)
	}
	defer conn.Close()

	queries := generated.New(conn)

	// создаём маршрутизатор
	r := setupRouter()

	// регистрируем маршруты
	r.GET("/api/subsc", ListSubscriptions(queries))
	r.GET("/api/subsc/:user_id", GetSubscriptionsFromUserId(queries))
	r.GET("/api/cost", TotalCostSubscription(queries))
	r.POST("/api/subsc", CreateSubscription(queries))
	r.PUT("/api/subsc/:id", UpdateSubscription(queries))
	r.DELETE("/api/subsc/:id", DeleteSubscription(queries))

	// запускаем сервер на порту 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server startup error")
	}
}
