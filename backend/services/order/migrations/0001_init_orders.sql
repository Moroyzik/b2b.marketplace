-- orders and order_items
CREATE TABLE IF NOT EXISTS orders (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  status VARCHAR(32),
  total_amount NUMERIC,
  currency VARCHAR(10),
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  shipping_info JSONB
);

CREATE TABLE IF NOT EXISTS order_items (
  id UUID PRIMARY KEY,
  order_id UUID NOT NULL,
  offer_id UUID NOT NULL,
  quantity INT NOT NULL,
  price NUMERIC,
  supplier_id UUID
);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);

