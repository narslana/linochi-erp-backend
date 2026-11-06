-- Local development only (Docker Compose runs this once, on an empty data
-- directory). It mirrors the cluster, where H1 creates the same roles with
-- DatabaseRole objects: an owner that owns the database and runs migrations, and
-- a restricted app role that the running app uses. The passwords are only valid
-- on this machine.
CREATE ROLE linochi_erp_owner LOGIN PASSWORD 'dev';
CREATE ROLE linochi_erp_app   LOGIN PASSWORD 'dev';
CREATE DATABASE linochi_erp OWNER linochi_erp_owner;
