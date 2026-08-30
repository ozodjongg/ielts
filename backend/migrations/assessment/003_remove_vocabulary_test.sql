-- Remove historical runtime data for the retired vocabulary_test assessment.
-- Shared dictionary/SRS data lives in the vocabulary schema and is untouched.
DELETE FROM anti_cheat_events
WHERE attempt_id IN (SELECT id FROM attempts WHERE service_code='vocabulary_test');

DELETE FROM attempts WHERE service_code='vocabulary_test';
DELETE FROM assignments WHERE service_code='vocabulary_test';
