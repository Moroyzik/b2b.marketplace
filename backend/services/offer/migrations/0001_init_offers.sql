-- offers
CREATE TABLE IF NOT EXISTS offers (
  id UUID PRIMARY KEY,
  product_id UUID NOT NULL,
  supplier_id UUID NOT NULL,
  price NUMERIC,
  currency VARCHAR(10),
  moq INT,
  in_stock INT,
  delivery_days INT,
  sku VARCHAR(255),
  lead_time TEXT,
  terms JSONB,
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_offers_product_id ON offers(product_id);
CREATE INDEX IF NOT EXISTS idx_offers_supplier_id ON offers(supplier_id);

