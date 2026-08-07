-- SPDX-License-Identifier: Apache-2.0
-- Explainability columns. A health officer receiving an alert must be able to
-- ask "why?" and get an answer from the same row that produced it.
--
-- exceedance is how rare the driver value is for that county and month against
-- the reference climatology (0.02 = the most extreme 2% of the last decade).
-- It describes the WEATHER, not a probability of an outbreak: this system has
-- no outbreak surveillance data and cannot estimate that. NULL for predictors
-- that do not compute it, such as the fixed-threshold rules engine.
ALTER TABLE risk_scores
    ADD COLUMN exceedance double precision
        CHECK (exceedance IS NULL OR (exceedance >= 0 AND exceedance <= 1)),
    ADD COLUMN explanation text;
