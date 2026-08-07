-- SPDX-License-Identifier: Apache-2.0
-- CASCADE: the postgis docker images pre-install dependent extensions
-- (postgis_topology, tiger geocoder) in the default database; a full
-- teardown must take them along.
DROP EXTENSION IF EXISTS postgis CASCADE;
