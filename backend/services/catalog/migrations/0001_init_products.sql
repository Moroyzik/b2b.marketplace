-- suppliers
CREATE TABLE IF NOT EXISTS suppliers (
  id UUID PRIMARY KEY,
  name VARCHAR(255),
  rating NUMERIC,
  contact JSONB,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- products
CREATE TABLE IF NOT EXISTS products (
  id UUID PRIMARY KEY,
  sku VARCHAR(255),
  title TEXT,
  description TEXT,
  category_id UUID,
  attributes JSONB,
  thumbnail_url TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  is_active BOOLEAN DEFAULT TRUE
);

CREATE INDEX IF NOT EXISTS idx_products_sku ON products(sku);
CREATE INDEX IF NOT EXISTS idx_products_title ON products USING GIN (to_tsvector('simple', title));

