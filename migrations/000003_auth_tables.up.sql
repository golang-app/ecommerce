BEGIN;

CREATE TABLE public.auth_customer (
	username varchar NOT NULL,
	password_hash varchar NOT NULL,
	CONSTRAINT auth_customer_pk PRIMARY KEY (username)
);

CREATE TABLE public.auth_session (
	id varchar NOT NULL,
	customer_id varchar NOT NULL,
	expires_at  timestamp with time zone NOT NULL
);

CREATE UNIQUE INDEX auth_session_id_idx ON public.auth_session (id);

COMMIT;
