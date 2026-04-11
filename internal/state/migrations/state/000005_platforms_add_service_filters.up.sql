ALTER TABLE platforms
ADD COLUMN service_filters_json TEXT NOT NULL DEFAULT '[]';
