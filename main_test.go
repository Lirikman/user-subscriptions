package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	generated "github.com/Lirikman/user-subscriptions/db/generated"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var db *pgxpool.Pool
var router *gin.Engine

func TestMain(m *testing.M) {
	ctx := context.Background()
	// запуск контейнера PostgreSQL
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "testdb",
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "secret",
		},
		WaitingFor: wait.ForExposedPort().WithStartupTimeout(60 * time.Second),
	}
	pgCont, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		log.Fatalf("failed to start container: %v", err)
	}
	defer func() {
		if err := pgCont.Terminate(ctx); err != nil {
			log.Fatalf("context termination error")
		}
	}()
	// получаем адрес и порт
	mappedPort, _ := pgCont.MappedPort(ctx, "5432")
	host, _ := pgCont.Host(ctx)
	// подключение к БД
	dsn := fmt.Sprintf("postgres://test:secret@%s:%s/testdb?sslmode=disable", host, mappedPort.Port())
	db, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer db.Close()
	// применение миграций
	_, err = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS subscriptions (id BIGSERIAL PRIMARY KEY, user_id UUID NOT NULL, service_name VARCHAR(255) NOT NULL, price INT NOT NULL, start_date DATE NOT NULL, end_date DATE DEFAULT NULL, CONSTRAINT unique_fields_with_null UNIQUE NULLS NOT DISTINCT (user_id, service_name, price, start_date, end_date));`)
	if err != nil {
		log.Fatalf("failed to create table subscriptions: %v", err)
	}

	// добавление тестовых данных в таблицу subscriptions
	_, err = db.Exec(ctx, "INSERT INTO subscriptions (user_id, service_name, price, start_date, end_date) VALUES ('3a51e2d6-b60b-40c5-993f-a251c9059bc6', 'okko', 550, '2025-10-01', '2025-11-01'), ('3a51e2d6-b60b-40c5-993f-a251c9059bc6', 'ivi', 400, '2025-11-01', '2026-01-01'), ('3a51e2d6-b60b-40c5-993f-a251c9059bc6', 'ivi', 550, '2026-01-01', NULL), ('66e8dcdd-aac8-436b-af34-eae57a4f0bca', 'filmix', 1200, '2026-01-01', '2027-01-01'), ('66e8dcdd-aac8-436b-af34-eae57a4f0bca', 'netflix', 2500, '2026-02-01', NULL), ('cb777ae7-4c84-4c1c-8440-346eb2bfbb75', 'megogo', 1100, '2025-10-01', NULL), ('cb777ae7-4c84-4c1c-8440-346eb2bfbb75', 'vk music', 400, '2026-01-01', '2026-03-01'), ('cb777ae7-4c84-4c1c-8440-346eb2bfbb75', 'yandex music', 900, '2026-03-01', '2026-05-01');")
	if err != nil {
		log.Fatalf("error adding data to table subscriptions: %v", err)
	}

	// инициализация Gin
	queries := generated.New(db)
	gin.SetMode(gin.TestMode)
	router = setupRouter()
	// регистрация маршрутов
	router.GET("/api/subsc", ListSubscriptions(queries))
	router.GET("/api/subsc/:user_id", GetSubscriptionsFromUserId(queries))
	router.GET("/api/cost", TotalCostSubscription(queries))
	router.POST("/api/subsc", CreateSubscription(queries))
	router.PUT("/api/subsc/:id", UpdateSubscription(queries))
	router.DELETE("/api/subsc/:id", DeleteSubscription(queries))
	os.Exit(m.Run())
}

func TestGetSubscriptionRight(t *testing.T) {
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodGet, "/api/subsc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusOK, w.Code)
	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 8, len(response))
}

func TestGetSubscriptionUserFromIDRight(t *testing.T) {
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodGet, "/api/subsc/cb777ae7-4c84-4c1c-8440-346eb2bfbb75", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusOK, w.Code)
	var response []map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := []map[string]any{{"user_id": "cb777ae7-4c84-4c1c-8440-346eb2bfbb75", "service_name": "megogo", "price": float64(1100), "start_date": "10-2025", "end_date": ""}, {"user_id": "cb777ae7-4c84-4c1c-8440-346eb2bfbb75", "service_name": "vk music", "price": float64(400), "start_date": "01-2026", "end_date": "03-2026"}, {"user_id": "cb777ae7-4c84-4c1c-8440-346eb2bfbb75", "service_name": "yandex music", "price": float64(900), "start_date": "03-2026", "end_date": "05-2026"}}
	assert.NoError(t, err)
	assert.Equal(t, 3, len(response))
	assert.Equal(t, want, response)
}

func TestGetSubscriptionUserFromIDWrong(t *testing.T) {
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodGet, "/api/subsc/55", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestCreateSubscriptionRight(t *testing.T) {
	// данные для запроса
	data := map[string]any{"user_id": "dfbd21b7-b27f-41ee-b41b-12078ac1035e", "service_name": "filmix", "price": 500, "start_date": "10-2025", "end_date": "12-2025"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPost, "/api/subsc", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusCreated, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"id": float64(9), "user_id": "dfbd21b7-b27f-41ee-b41b-12078ac1035e", "service_name": "filmix", "price": float64(500), "start_date": "10-2025", "end_date": "12-2025"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestCreateSubscriptionWrong1(t *testing.T) {
	// данные для запроса (неккорректный user_id)
	data := map[string]any{"user_id": "dfbd21b7", "service_name": "filmix", "price": 500, "start_date": "10-2025", "end_date": "12-2025"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPost, "/api/subsc", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestCreateSubscriptionWrong2(t *testing.T) {
	// данные для запроса (отсутствует service_name)
	data := map[string]any{"user_id": "dfbd21b7-b27f-41ee-b41b-12078ac1035e", "price": 500, "start_date": "10-2025", "end_date": "12-2025"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPost, "/api/subsc", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "invalid request"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestCreateSubscriptionWrong3(t *testing.T) {
	// данные для запроса (некорректная цена)
	data := map[string]any{"user_id": "dfbd21b7-b27f-41ee-b41b-12078ac1035e", "service_name": "filmix", "price": -1500, "start_date": "10-2025", "end_date": "12-2025"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPost, "/api/subsc", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "subscription price must be positive"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestCreateSubscriptionWrong4(t *testing.T) {
	// данные для запроса (некорректная дата начала подписки)
	data := map[string]any{"user_id": "dfbd21b7-b27f-41ee-b41b-12078ac1035e", "service_name": "filmix", "price": 500, "start_date": "10", "end_date": "12-2025"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPost, "/api/subsc", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "subscription start date is incorrect (example start date: 07-2025)"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestCreateSubscriptionWrong5(t *testing.T) {
	// данные для запроса (некорректная дата окончания подписки)
	data := map[string]any{"user_id": "dfbd21b7-b27f-41ee-b41b-12078ac1035e", "service_name": "filmix", "price": 500, "start_date": "10-2025", "end_date": "2025"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPost, "/api/subsc", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "subscription end date is incorrect (example: 05-2026)"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestCreateSubscriptionWrong6(t *testing.T) {
	// данные для запроса (некорректно задан период подписки)
	data := map[string]any{"user_id": "dfbd21b7-b27f-41ee-b41b-12078ac1035e", "service_name": "filmix", "price": 500, "start_date": "10-2025", "end_date": "10-2024"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPost, "/api/subsc", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "the subscription period is set incorrectly"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestCreateSubscriptionWrong7(t *testing.T) {
	// данные для запроса (содаём копию записи)
	data := map[string]any{"user_id": "cb777ae7-4c84-4c1c-8440-346eb2bfbb75", "service_name": "megogo", "price": 1100, "start_date": "10-2025", "end_date": ""}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPost, "/api/subsc", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "internal server error"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestUpdateSubscriptionRight(t *testing.T) {
	// данные для запроса
	data := map[string]any{"user_id": "81997f52-03eb-42ac-89d8-e55d26b09003", "service_name": "kinozal", "price": 120, "start_date": "05-2025", "end_date": "05-2026"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPut, "/api/subsc/8", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"id": float64(8), "user_id": "81997f52-03eb-42ac-89d8-e55d26b09003", "service_name": "kinozal", "price": float64(120), "start_date": "05-2025", "end_date": "05-2026"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestUpdateSubscriptionWrong1(t *testing.T) {
	// данные для запроса (несуществующий id записи)
	data := map[string]any{"user_id": "81997f52-03eb-42ac-89d8-e55d26b09003", "service_name": "kinozal", "price": 120, "start_date": "05-2025", "end_date": "05-2026"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPut, "/api/subsc/55", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusNotFound, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "no records with this ID were found"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestUpdateSubscriptionWrong2(t *testing.T) {
	// данные для запроса (некорректный user_id)
	data := map[string]any{"user_id": "03eb", "service_name": "kinozal", "price": 120, "start_date": "05-2025", "end_date": "05-2026"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPut, "/api/subsc/8", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "user id is incorrect (example user_id: 123e4567-e89b-12d3-a456-426655440000)"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestUpdateSubscriptionWrong3(t *testing.T) {
	// данные для запроса (пустое поле service_name)
	data := map[string]any{"user_id": "81997f52-03eb-42ac-89d8-e55d26b09003", "service_name": "", "price": 120, "start_date": "05-2025", "end_date": "05-2026"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPut, "/api/subsc/8", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "invalid request"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestUpdateSubscriptionWrong4(t *testing.T) {
	// данные для запроса (некорректная цена)
	data := map[string]any{"user_id": "81997f52-03eb-42ac-89d8-e55d26b09003", "service_name": "kinozal", "price": -500, "start_date": "05-2025", "end_date": "05-2026"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPut, "/api/subsc/8", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "subscription price must be positive"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestUpdateSubscriptionWrong5(t *testing.T) {
	// данные для запроса (некорректная дата начала подписки)
	data := map[string]any{"user_id": "81997f52-03eb-42ac-89d8-e55d26b09003", "service_name": "kinozal", "price": 500, "start_date": "25", "end_date": "05-2026"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPut, "/api/subsc/8", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "subscription start date is incorrect (example start date: 07-2025)"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestUpdateSubscriptionWrong6(t *testing.T) {
	// данные для запроса (некорректная дата окончания подписки)
	data := map[string]any{"user_id": "81997f52-03eb-42ac-89d8-e55d26b09003", "service_name": "kinozal", "price": 500, "start_date": "03-2025", "end_date": "1"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPut, "/api/subsc/8", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "subscription end date is incorrect (example: 05-2026)"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestUpdateSubscriptionWrong7(t *testing.T) {
	// данные для запроса (некорректно задан период подписки)
	data := map[string]any{"user_id": "81997f52-03eb-42ac-89d8-e55d26b09003", "service_name": "kinozal", "price": 500, "start_date": "03-2025", "end_date": "10-2022"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodPut, "/api/subsc/8", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "subscription period is set incorrectly"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestTotalCostSubscription(t *testing.T) {
	// данные для запроса
	data := map[string]any{"user_id": "3a51e2d6-b60b-40c5-993f-a251c9059bc6", "service_name": "ivi", "start_date": "10-2025", "end_date": "03-2026"}
	reqData, _ := json.Marshal(data)
	// выполнение запроса
	req, _ := http.NewRequest(http.MethodGet, "/api/cost", bytes.NewBuffer(reqData))
	// добавляем заголовок
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"message": "total price - 950 rubles"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestDeleteSubscriptionRight(t *testing.T) {
	// выполнение запроса удаления записи
	reqDel, _ := http.NewRequest(http.MethodDelete, "/api/subsc/8", nil)
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, reqDel)
	// проверка результатов
	assert.Equal(t, http.StatusOK, wDel.Code)
	var responseDel map[string]any
	err := json.Unmarshal(wDel.Body.Bytes(), &responseDel)
	want := map[string]any{"message": "entry with ID 8 has been successfully deleted"}
	assert.NoError(t, err)
	assert.Equal(t, want, responseDel)
	// выполнение запроса получения оставшихся записей
	reqGet, _ := http.NewRequest(http.MethodGet, "/api/subsc", nil)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)
	// проверка результатов
	assert.Equal(t, http.StatusOK, wGet.Code)
	var responseGet []map[string]any
	err = json.Unmarshal(wGet.Body.Bytes(), &responseGet)
	assert.NoError(t, err)
	assert.Equal(t, 8, len(responseGet))
}

func TestDeleteSubscriptionWrong(t *testing.T) {
	// данные для запроса (несуществющий id записи)
	req, _ := http.NewRequest(http.MethodDelete, "/api/subsc/38", nil)
	// добавляем заголовок
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// проверка результатов
	assert.Equal(t, http.StatusNotFound, w.Code)
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	want := map[string]any{"error": "no records with this ID were found"}
	assert.NoError(t, err)
	assert.Equal(t, want, response)
}
