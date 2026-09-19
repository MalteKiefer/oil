CREATE TABLE consumption_types (
  id             INTEGER PRIMARY KEY,
  name           TEXT UNIQUE NOT NULL,
  unit           TEXT NOT NULL,
  price_per_unit REAL,
  tank_size      REAL
);

CREATE TABLE readings (
  id            INTEGER PRIMARY KEY,
  type_id       INTEGER NOT NULL REFERENCES consumption_types(id),
  reading_date  TEXT NOT NULL,
  counter_value REAL NOT NULL,
  heating_mode  TEXT,
  created_at    TEXT NOT NULL,
  UNIQUE(type_id, reading_date)
);

CREATE TABLE refills (
  id          INTEGER PRIMARY KEY,
  type_id     INTEGER NOT NULL REFERENCES consumption_types(id),
  refill_date TEXT NOT NULL,
  amount      REAL NOT NULL,
  created_at  TEXT NOT NULL
);
