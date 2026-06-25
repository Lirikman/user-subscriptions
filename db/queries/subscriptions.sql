-- name: GetSubscriptions :many
SELECT user_id, service_name, price, TO_CHAR(start_date, 'MM-YYYY') AS start_date,  COALESCE(TO_CHAR(end_date, 'MM-YYYY'), '') AS end_date
FROM subscriptions
WHERE user_id = $1;

-- name: ListSubscriptions :many
SELECT id, user_id, service_name, price, TO_CHAR(start_date, 'MM-YYYY') AS start_date, COALESCE(TO_CHAR(end_date, 'MM-YYYY'), '') AS end_date
FROM subscriptions
ORDER BY id;

-- name: CreateSubscription :one
INSERT INTO subscriptions (
user_id, service_name, price, start_date, end_date
) VALUES (
$1, $2, $3, $4, $5
)
RETURNING id, user_id, service_name, price, TO_CHAR(start_date, 'MM-YYYY') AS start_date, COALESCE(TO_CHAR(end_date, 'MM-YYYY'), '') AS end_date;

-- name: UpdateSubscription :one
UPDATE subscriptions
SET user_id = $2, service_name = $3, price = $4, start_date = $5, end_date = $6
WHERE id = $1
RETURNING id, user_id, service_name, price, TO_CHAR(start_date, 'MM-YYYY') AS start_date, COALESCE(TO_CHAR(end_date, 'MM-YYYY'), '') AS end_date;

-- name: DeleteSubscription :exec
DELETE FROM subscriptions
WHERE id = $1;

-- name: TotalPriceSubscription :one
SELECT COALESCE(SUM(price), 0) AS total_price
FROM subscriptions
WHERE user_id = @user_id
  AND service_name = @service_name
  AND start_date <= @end_date 
  AND (end_date >= @start_date OR end_date IS NULL);

-- name: CheckingServiceName :one
SELECT service_name
FROM subscriptions
WHERE service_name = $1;

-- name: GetSubscriptionFromID :one
SELECT * 
FROM subscriptions
WHERE id = $1;