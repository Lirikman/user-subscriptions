-- +goose Up
CREATE TABLE IF NOT EXISTS subscriptions (
	id BIGSERIAL PRIMARY KEY,
	user_id UUID NOT NULL,
	service_name VARCHAR(255) NOT NULL,
	price INT NOT NULL,
	start_date DATE NOT NULL,
   	end_date DATE DEFAULT NULL,
	CONSTRAINT unique_fields_with_null UNIQUE NULLS NOT DISTINCT (user_id, service_name, price, start_date, end_date)
);

-- +goose Down
DROP TABLE IF EXISTS subscriptions;
