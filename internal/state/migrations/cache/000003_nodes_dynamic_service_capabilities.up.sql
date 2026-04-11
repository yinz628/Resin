ALTER TABLE nodes_dynamic
ADD COLUMN supports_openai INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes_dynamic
ADD COLUMN supports_anthropic INTEGER NOT NULL DEFAULT 0;
