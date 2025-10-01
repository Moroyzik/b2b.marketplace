-- carts and items
CREATE TABLE IF NOT EXISTS carts (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cart_items (
  id UUID PRIMARY KEY,
  cart_id UUID NOT NULL,
  offer_id UUID NOT NULL,
  quantity INT NOT NULL,
  price_at_add NUMERIC,
  metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_carts_user_id ON carts(user_id);
CREATE INDEX IF NOT EXISTS idx_cart_items_cart_id ON cart_items(cart_id);

