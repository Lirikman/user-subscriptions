# USER-SUBSCRIPTIONS


## Description
REST-service for aggregating data on user subscriptions

Example of 'Subscriptions' object:
* **id (bigserial, read-only):** Unique record number
* **user_id (uuid, required):** Unique user identifier
* **service_name (string, required):** Name of the service providing the subscription
* **price (int, required):** Monthly subscription cost in rubles
* **start_date (date, required):** Subscription start date (month - year)
* **end_date (date, optional):** Subscription end date


## 🛠️ Tech Stack

### Core Stack
* **Language:** **Go (Golang)**
* **Web Framework:** **Gin Gonic**
* **Database:** **PostgreSQL**

### Database & Development Tools
* **Data Access Layer:** **sqlc**
* **Database Migrations:** **goose**

### API & Environment
* **API Documentation:** **Swagger (OpenAPI 3.0)**
* **Containerization:** **Docker Compose** 

## 💻 Prerequisites
To start the project you will need:
 * Docker Engine v24.0+
 * Docker Compose v2.20+

## 🚀 Quick Start
Follow these steps to run the application locally:

1. Clone the repository
```bash
git clone https://github.com/Lirikman/user-subscriptions.git
cd user-subscriptions
```

2. Launch the project
```bash
make run
```

3. Check the work
After a successful launch, the application will be available at: 
👉 http://127.0.0.1:8080/api

4. Stopping and removing the container
```bash
make stop
```


## 🛠️ Development

### 🧪 Run golangci-lint 

```bash
make lint
```

### 🧪 Run tests

```bash
make test
```

## 📡 API documentation

📝 **Swagger UI (Interactive documentation):** 
http://localhost:8080/swagger/index.html

All requests are sent to the base URL: http://127.0.0.1:8080/api

Headers: Content-Type: application/json


### Getting all user subscription records
Returns all user subscription records.

**GET** /subsc

**Example answer:**
```json
[
  {
    "id": 1,
    "user_id": "81997f52-03eb-42ac-89d8-e55d26b09003",
    "service_name": "yandex music",
    "price": 350,
    "start_date": "07-2025",
    "end_date": "09-2025"
  },
  {
    "id": 2,
    "user_id": "81997f52-03eb-42ac-89d8-e55d26b09003",
    "service_name": "vk music",
    "price": 500,
    "start_date": "08-2025",
    "end_date": "09-2025"
  },
  {
    "id": 3,
    "user_id": "92758591-6720-44de-a97a-3bb1d00a961a",
    "service_name": "ivi",
    "price": 550,
    "start_date": "11-2025",
    "end_date": "12-2025"
  }
]
```
Response code: 200 OK

### Creating a new subscription entry
Creates a new user subscription record.

- All records are unique.
- The 'user_id' field must be in the UUID format, not empty.
- The 'price' field is a positive integer, not empty.
- The 'service name' field - string, not empty.
- The 'end_date' field is optional.
- The format of the 'start_date' and 'end_date' fields is month-year (example, '05-2022').

**POST** /subsc

**Request body example:**
```json
{
  "user_id":"81997f52-03eb-42ac-89d8-e55d26b09003",
  "service_name":"yandex music",
  "price":350,
  "start_date":"07-2025",
  "end_date":"09-2025"
}
```

or

```json
{
  "user_id":"81997f52-03eb-42ac-89d8-e55d26b09003",
  "service_name":"ivi",
  "price":400,
  "start_date":"05-2025"
}
```

**Example answer:**
```json
{
  "id": 5,
  "user_id":"81997f52-03eb-42ac-89d8-e55d26b09003",
  "service_name":"kinopoisk",
  "price":1200,
  "start_date":"05-2025",
  "end_date":"09-2025"
}
```
Response code: 201 Created

### Getting subscriptions by user id
Returns all subscriptions of a user by the specified user id.

**GET** /subsc/:user_id

**Example answer:**
```json
[
  {
    "user_id":"81997f52-03eb-42ac-89d8-e55d26b09003",
    "service_name":"ivi",
    "price":400,
    "start_date":"05-2025",
    "end_date":""
  },
  {
    "user_id":"81997f52-03eb-42ac-89d8-e55d26b09003", 
    "service_name":"kinozal",
    "price":400,
    "start_date":"08-2025",
    "end_date":"10-2025"
  }
]
```
Response code: 200 OK

### Update subscription information
Updating user subscription information by record ID number.

**PUT** /subsc/5

**Request body example:**
```json
{
  "user_id":"81997f52-03eb-42ac-89d8-e55d26b09003",
  "service_name":"vk music",
  "price":500,
  "start_date":"02-2025",
  "end_date":"05-2025"
}
```

**Example answer:**
```json
{
  "id": 5,
  "user_id":"81997f52-03eb-42ac-89d8-e55d26b09003",
  "service_name":"vk music",
  "price":500,
  "start_date":"02-2025",
  "end_date":"05-2025"
}
```
Response code: 200 OK

### Deleting a subscription entry
Deletes subscription records by the specified record ID number.

**DELETE** /subsc/10

**Example answer:**
```
  "entry with ID 10 has been successfully deleted"
```
Response code: 200 OK

### Total cost of subscriptions by period
Returns the sum of the prices of all user subscriptions for the specified period.

**POST** /cost

**Request body example:**
```json
{
  "user_id":"81997f52-03eb-42ac-89d8-e55d26b09003",
  "service_name":"yandex music",
  "start_date":"02-2025",
  "end_date":"10-2025"
}
```

**Example answer:**
```
  "total price - 750 rubles"
```
Response code: 200 OK
