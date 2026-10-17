-- One driver profile per Auth Service user. Fails if existing rows already share a user_id.
CREATE UNIQUE INDEX drivers_user_id_key ON drivers (user_id);
