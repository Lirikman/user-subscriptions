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

-- name: CheckingServiceName :one
SELECT service_name
FROM subscriptions
WHERE service_name = $1;

-- name: GetSubscriptionFromID :one
SELECT * 
FROM subscriptions
WHERE id = $1;

-- name: TotalPriceSubscription :one
WITH period_bounds AS (
    SELECT
        GREATEST(start_date, $3::DATE) AS active_start,
        LEAST(COALESCE(end_date, CURRENT_DATE), $4::DATE) AS active_end,
        price
    FROM 
        subscriptions
    WHERE 
        user_id = $1
        AND service_name = $2
        AND start_date <= $4::DATE
        AND COALESCE(end_date, CURRENT_DATE) >= $3::DATE
),
calculated_months AS (
    SELECT 
        price,
        EXTRACT(YEAR FROM AGE(active_end, active_start)) * 12 + EXTRACT(MONTH FROM AGE(active_end, active_start)) AS months_count
    FROM 
        period_bounds
)
SELECT 
    COALESCE(SUM(price * months_count), 0)::BIGINT AS total_cost
FROM 
    calculated_months;