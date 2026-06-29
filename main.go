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
	_ "github.com/Lirikman/user-subscriptions/docs"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	config.AllowHeaders = []string{"Origin", "Content-Type"}
	router.Use(cors.New(config))
	// подключаем инструмент восстановления сбоев
	router.Use(gin.Recovery())
	// подключаем логгер
	router.Use(gin.Logger())
	return router
}

// DTO для запроса создания записи
type CreateSubReqDTO struct {
	UserID      string `json:"user_id" binding:"required" example:"9c7ae5f1-f950-4790-8201-2b45d7853bd7"`
	ServiceName string `json:"service_name" binding:"required" example:"kion"`
	Price       int32  `json:"price" binding:"required" example:"500"`
	StartDate   string `json:"start_date" binding:"required" example:"05-2025"`
	EndDate     string `json:"end_date" example:"07-2025"`
}

// DTO для входящего обновления записи
type UpdateSubReqDTO struct {
	UserID      string `json:"user_id" binding:"required" example:"a4fe5fe8-852e-4d29-9b34-5e8f84d18aea"`
	ServiceName string `json:"service_name" binding:"required" example:"okko"`
	Price       int32  `json:"price" binding:"required" example:"650"`
	StartDate   string `json:"start_date" binding:"required" example:"06-2024"`
	EndDate     string `json:"end_date" example:"10-2024"`
}

// DTO для получения общей стоимости подписок
type TotalPriceReqDTO struct {
	UserID      string `json:"user_id" binding:"required" example:"81997f52-03eb-42ac-89d8-e55d26b09003"`
	ServiceName string `json:"service_name" binding:"required" example:"ivi"`
	StartDate   string `json:"start_date" binding:"required" example:"02-2025"`
	EndDate     string `json:"end_date" binding:"required" example:"10-2025"`
}

// Структура ошибки при запросах
type HTTPError struct {
	Code    int    `json:"code" example:"400"`
	Message string `json:"message" example:"invalid request"`
}

// CreateSubscription godoc
// @Summary      Creating a new subscription
// @Description  Creates a new subscription record in the database with validation of dates and prices
// @Tags         subscriptions
// @Accept       json
// @Produce      json
// @Param        request  body      CreateSubReqDTO  true  "Data for creating a subscription"
// @Success      201      {object}  generated.CreateSubscriptionRow "Subscription successfully created"
// @Failure      400      {object}  HTTPError "Data validation error"
// @Failure      500      {object}  HTTPError "Internal Server Error"
// @Router       /subsc [post]
func CreateSubscription(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateSubReqDTO

		// десериализация данных тела запроса
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "invalid request"})
			return
		}

		// проверяем user_id на корректность
		var userUUID pgtype.UUID
		err := userUUID.Scan(req.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"})
			return
		}

		// проверяем корректность ввода цены
		if req.Price <= 0 {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription price must be positive"})
			return
		}
		// проверяем корректность ввода периода подписки
		var endDate pgtype.Date
		layout := "01-2006" // шаблон формата времени
		parsStrDate, err := time.Parse(layout, req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription start date is incorrect (example start date: 07-2025)"})
			return
		}
		if req.EndDate != "" {
			parsEndDate, err := time.Parse(layout, req.EndDate)
			if err != nil {
				c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription end date is incorrect (example: 05-2026)"})
				return
			}
			if parsStrDate.Compare(parsEndDate) > 0 {
				c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "the subscription period is set incorrectly"})
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
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}
		c.JSON(http.StatusCreated, subsc)
	}
}

// ListSubscriptions godoc
// @Summary      Getting all subscriptions
// @Description  Returns a full list of subscription records. If there are no subscriptions, returns a 404 response
// @Tags         subscriptions
// @Accept       json
// @Produce      json
// @Success      200  {array}   generated.ListSubscriptionsRow "Successful response with subscription list"
// @Failure      404  {object}  HTTPError      "No subscriptions found"
// @Failure      500  {object}  HTTPError      "Internal Server Error"
// @Router       /subsc [get]
func ListSubscriptions(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		subsrciptions, err := db.ListSubscriptions(c)
		if err != nil {
			// ошибка сервера
			log.Printf("error getting list of user subscriptions: %v\n", err)
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}
		// проверяем, вызвана ли ошибка отсутствием записей в БД
		if len(subsrciptions) == 0 {
			c.JSON(http.StatusNotFound, HTTPError{Code: http.StatusNotFound, Message: "no subscriptions found"})
			return
		}
		c.JSON(http.StatusOK, subsrciptions)
	}
}

// GetSubscriptionsFromUserId godoc
// @Summary      Getting a subscription record by user ID
// @Description  Returning information about all user subscriptions by user_id
// @Tags         subscriptions
// @Accept       json
// @Produce      json
// @Param        user_id   path      string  true  "User ID"  format(uuid)  example(123e4567-e89b-12d3-a456-426614174000)
// @Success      200       {array}   generated.GetSubscriptionsRow "Successfully retrieved records"
// @Failure      400       {object}  HTTPError "Incorrect user_id"
// @Failure      404       {object}  HTTPError "Database entry not found"
// @Failure      500       {object}  HTTPError "Internal Server Error"
// @Router       /subsc/{user_id} [get]
func GetSubscriptionsFromUserId(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var userUUID pgtype.UUID
		// чтение параметра user_id из запроса
		userIDStr := c.Param("user_id")

		// проверяем user_id на корректность
		err := userUUID.Scan(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"})
			return
		}

		// получем все записи пользователя о подписках
		subsc, err := db.GetSubscriptions(c, userUUID)
		if err != nil {
			// ошибка сервера
			log.Printf("error retrieving user subscriptions: %v\n", err)
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}
		// проверяем, вызвана ли ошибка отсутствием записи в БД
		if len(subsc) == 0 {
			c.JSON(http.StatusNotFound, HTTPError{Code: http.StatusNotFound, Message: "user with this user_id not found"})
			return
		}
		c.JSON(http.StatusOK, subsc)
	}
}

// UpdateSubscription godoc
// @Summary      Updating a subscription record
// @Description  Updates the data of an existing subscription by its ID with full parameter validation
// @Tags         subscriptions
// @Accept       json
// @Produce      json
// @Param        id        path      int  true  "Record id (ID)"
// @Param        request   body      UpdateSubReqDTO  true  "New subscription data"
// @Success      200       {object}  generated.UpdateSubscriptionRow "Successful updated"
// @Failure      400       {object}  HTTPError "Data validation error"
// @Failure      404       {object}  HTTPError "Database entry not found"
// @Failure      500       {object}  HTTPError "Internal Server Error"
// @Router       /subsc/{id} [put]
func UpdateSubscription(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req UpdateSubReqDTO
		var userUUID pgtype.UUID

		// чтение параметра id из запроса
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "incorrect ID value entered"})
			return
		}
		// проверяем наличие записи в БД
		_, err = db.GetSubscriptionFromID(c, id)
		if err != nil {
			// проверяем, вызвана ли ошибка отсутствием строки в БД
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, HTTPError{Code: http.StatusNotFound, Message: "no records with this ID were found"})
				return
			}
			// иначе это другая ошибка сервера
			log.Printf("error retrieving a record from the database by ID: %v\n", err)
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}

		// десериализация данных тела запроса
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "invalid request"})
			return
		}

		// проверяем user_id на корректность
		err = userUUID.Scan(req.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"})
			return
		}

		// проверяем корректность ввода цены
		if req.Price <= 0 {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription price must be positive"})
			return
		}

		// проверяем корректность ввода периода подписки
		var endDate pgtype.Date
		layout := "01-2006" // шаблон формата времени
		parsStrDate, err := time.Parse(layout, req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription start date is incorrect (example start date: 07-2025)"})
			return
		}
		if req.EndDate != "" {
			parsEndDate, err := time.Parse(layout, req.EndDate)
			if err != nil {
				c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription end date is incorrect (example: 05-2026)"})
				return
			}
			if parsStrDate.Compare(parsEndDate) > 0 {
				c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription period is set incorrectly"})
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
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}
		c.JSON(http.StatusOK, res)
	}
}

// DeleteSubscription godoc
// @Summary      Deleting a record
// @Description  Removes a subscription record from the database by its unique ID
// @Tags         subscriptions
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "ID number of the entry in the database (id)"
// @Success      200  {string}  string "Successful removal"
// @Failure      400  {object}  HTTPError "Invalid ID format"
// @Failure      404  {object}  HTTPError "Database entry not found"
// @Failure      500  {object}  HTTPError "Internal Server Error"
// @Router       /subsc/{id} [delete]
func DeleteSubscription(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		// чтение параметра id из запроса
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "incorrect ID value entered"})
			return
		}
		// проверяем наличие записи в БД
		_, err = db.GetSubscriptionFromID(c, id)
		if err != nil {
			// проверяем, вызвана ли ошибка отсутствием строки в БД
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, HTTPError{Code: http.StatusNotFound, Message: "no records with this ID were found"})
				return
			}
			// иначе это другая ошибка сервера
			log.Printf("error retrieving a record from the database by ID: %v\n", err)
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}

		// удаляем запись
		err = db.DeleteSubscription(c, id)
		if err != nil {
			log.Printf("error deleting user subscription: %v\n", err)
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}
		message := fmt.Sprintf("entry with ID %s has been successfully deleted", idStr)
		c.String(http.StatusOK, message)
	}
}

// TotalCostSubscription godoc
// @Summary      Total cost of subscriptions for the period
// @Description  The total cost of all user subscriptions by service name and period is returned
// @Tags         subscriptions
// @Accept       json
// @Produce      json
// @Param        request   body      TotalPriceReqDTO  true  "Data for requesting the total cost"
// @Success      200       {string}  string "Successful receipt of the total cost"
// @Failure      400       {object}  HTTPError "Data validation error"
// @Failure      404       {object}  HTTPError "Database entry not found"
// @Failure      500       {object}  HTTPError "Internal Server Error"
// @Router       /cost [post]
func TotalCostSubscription(db *generated.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TotalPriceReqDTO
		var userUUID pgtype.UUID

		// десериализация данных тела запроса
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "invalid request"})
			return
		}
		// чтение параметра user_id из запроса
		userIDStr := req.UserID

		// проверяем user_id на корректность
		err := userUUID.Scan(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"})
			return
		}

		// проверка наличия записи в БД
		subsc, err := db.GetSubscriptions(c, userUUID)
		if err != nil {
			// ошибка сервера
			log.Printf("error retrieving user subscriptions: %v\n", err)
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}
		// проверяем, вызвана ли ошибка отсутствием строки в БД
		if len(subsc) == 0 {
			c.JSON(http.StatusNotFound, HTTPError{Code: http.StatusNotFound, Message: "user with this user ID will not be found"})
			return
		}

		// проверка наличия сервиса в БД
		svcName, err := db.CheckingServiceName(c, req.ServiceName)
		if err != nil {
			log.Printf("checking service name error: %v\n", err)
			c.JSON(http.StatusNotFound, HTTPError{Code: http.StatusNotFound, Message: "service name you entered was not found"})
			return
		}

		// проверка корректности ввода периода подписки
		layout := "01-2006" // шаблон формата времени
		parsStrDate, err := time.Parse(layout, req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription start date is incorrect (example start date: 07-2025)"})
			return
		}
		parsEndDate, err := time.Parse(layout, req.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "subscription end date is incorrect (example end date: 05-2026)"})
			return
		}
		if parsStrDate.Compare(parsEndDate) > 0 {
			c.JSON(http.StatusBadRequest, HTTPError{Code: http.StatusBadRequest, Message: "the subscription period is set incorrectly"})
			return
		}
		strDate := pgtype.Date{Time: parsStrDate, Valid: true}
		endDate := pgtype.Date{Time: parsEndDate, Valid: true}

		// формируем параметры для запроса
		params := generated.TotalPriceSubscriptionParams{
			UserID:      userUUID,
			ServiceName: svcName,
			Column3:     strDate,
			Column4:     endDate,
		}
		res, err := db.TotalPriceSubscription(c, params)
		if err != nil {
			log.Printf("error getting the total cost of subscriptions: %v\n", err)
			c.JSON(http.StatusInternalServerError, HTTPError{Code: http.StatusInternalServerError, Message: "internal server error"})
			return
		}
		message := fmt.Sprintf("total price - %v rubles", res)
		c.String(http.StatusOK, message)
	}
}

// @title           User subscription
// @version         1.0
// @description     REST-API service for aggregating data on user subscriptions
// @host            localhost:8080
// @BasePath        /api
func main() {
	// загружаем переменные окружения
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
	// освобождаем ресурсы
	defer func() {
		if closeErr := db.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
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
	r.POST("/api/cost", TotalCostSubscription(queries))
	r.POST("/api/subsc", CreateSubscription(queries))
	r.PUT("/api/subsc/:id", UpdateSubscription(queries))
	r.DELETE("/api/subsc/:id", DeleteSubscription(queries))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// запускаем сервер на порту 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server startup error")
	}
}
