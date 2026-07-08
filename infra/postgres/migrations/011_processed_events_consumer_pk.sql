-- 011_processed_events_consumer_pk.sql
-- Allow multiple consumers to process the same event_id independently.

ALTER TABLE processed_events DROP CONSTRAINT IF EXISTS processed_events_pkey;

ALTER TABLE processed_events
  ADD CONSTRAINT processed_events_pkey PRIMARY KEY (event_id, consumer);
