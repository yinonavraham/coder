## Recommendation

For production deployments, we recommend using an external [PostgreSQL](https://www.postgresql.org/) database (version 13 or higher).

## Basic configuration

Before starting the Coder server, prepare the database server by creating a role and a database.
Remember that the role must have access to the created database.

With `psql`:

```sql
CREATE ROLE coder LOGIN SUPERUSER PASSWORD 'secret42';
```

With `psql -U coder`:

```sql
CREATE DATABASE coder;
```

Coder configuration is defined via [environment variables](../admin/configure.md).
The database client requires the connection string provided via the `CODER_PG_CONNECTION_URL` variable.

```console
export CODER_PG_CONNECTION_URL="postgres://coder:secret42@localhost/coder?sslmode=disable"
```

## Custom schema

For installations with elevated security requirements, it's advised to use a separate [schema](https://www.postgresql.org/docs/current/ddl-schemas.html) instead of the public one.

With `psql -U coder`:

```sql
CREATE SCHEMA myschema;
```

Once the schema is created, you can list all schemas with `\dn`:

```
     List of schemas
     Name  |  Owner
-----------+----------
 myschema  | coder
 public    | postgres
(2 rows)
```

In this case the database client requires the modified connection string:

```console
export CODER_PG_CONNECTION_URL="postgres://coder:secret42@localhost/coder?sslmode=disable&search_path=myschema"
```

The `search_path` parameter determines the order of schemas in which they are visited while looking for a specific table.
The first schema named in the search path is called the current schema. By default `search_path` defines the following schemas:

```sql
SHOW search_path;

search_path
--------------
 "$user", public
```

Using the `search_path` in the connection string corresponds to the following `psql` command:

```sql
ALTER ROLE coder SET search_path = myschema;
```

## Migrating from the built-in database

If you installed Coder on a VM and would like to migrate your built-in database, you can dump the database and restore on the external database.

```sh
# Get the built in Postgres URL for the "coder" user
export CODER_BUILT_IN_DB_URL=$(sudo -u coder /bin/bash -c 'coder server postgres-builtin-url --raw-url')

# Dump the "coder" database to coder.sql (PostgreSQL client required)
pg_dump $CODER_BUILT_IN_DB_URL --no-publications --no-owner > coder.sql
```

To 

```sh
export CODER_EXTERNAL_DB_URL=postgres://coder:secret42@localhost/coder?sslmode=disable
pg_restore -d $CODER_EXTERNAL_DB_URL coder.sql

psql $CODER_EXTERNAL_DB_URL -f coder.sql

SELECT 'ALTER TABLE public.' || table_name || ' SET SCHEMA myschema;' FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE';



psq
```

## Troubleshooting

### Coder server fails startup with "current_schema: converting NULL to string is unsupported"

Please make sure that the schema selected in the connection string `...&search_path=myschema` exists
and the role has granted permissions to access it. The schema should be present on this listing:

```console
psql -U coder -c '\dn'
```

## Next steps

- [Configuring Coder](../admin/configure.md)
- [Templates](../templates/README.md)
