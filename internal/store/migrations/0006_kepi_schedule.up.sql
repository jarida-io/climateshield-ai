-- SPDX-License-Identifier: Apache-2.0
-- Core KEPI (Kenya Expanded Programme on Immunization) routine schedule:
-- birth, 6/10/14 weeks, 9 months, 18 months. due_age_days is age at which the
-- dose is due; a dose becomes overdue overdue_grace_days after that (grace
-- period is an implementation assumption, recorded in NOTES.md).
CREATE TABLE vaccine_schedule (
    code text PRIMARY KEY,
    name text NOT NULL,
    due_age_days int NOT NULL,
    overdue_grace_days int NOT NULL DEFAULT 14
);

INSERT INTO vaccine_schedule (code, name, due_age_days) VALUES
    ('bcg', 'BCG', 0),
    ('opv0', 'OPV birth dose', 0),
    ('opv1', 'OPV 1', 42),
    ('dpt1', 'DPT-HepB-Hib 1', 42),
    ('pcv1', 'PCV 1', 42),
    ('rota1', 'Rotavirus 1', 42),
    ('opv2', 'OPV 2', 70),
    ('dpt2', 'DPT-HepB-Hib 2', 70),
    ('pcv2', 'PCV 2', 70),
    ('rota2', 'Rotavirus 2', 70),
    ('opv3', 'OPV 3', 98),
    ('dpt3', 'DPT-HepB-Hib 3', 98),
    ('pcv3', 'PCV 3', 98),
    ('ipv', 'IPV', 98),
    ('mr1', 'Measles-Rubella 1', 270),
    ('mr2', 'Measles-Rubella 2', 540);
