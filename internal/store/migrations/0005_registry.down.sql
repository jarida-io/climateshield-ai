-- SPDX-License-Identifier: Apache-2.0
DROP TRIGGER immunization_events_append_only ON immunization_events;
DROP FUNCTION forbid_immunization_mutation();
DROP TABLE immunization_events;
DROP TABLE consent_log;
DROP TABLE children;
DROP TABLE guardians;
