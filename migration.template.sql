################################################################################
## !!! Don't forget connect to database source, uncomment:
#connect source
## Source may be a source name from configuration file
## Or it a connect string in format:
#connect Driver://user:password@host[:port]/dbname
################################################################################
## Requests must be separated by ';' delimeter
#select sysdate from dual;
################################################################################
## Use '/' for delimeter PL/SQL code, begin end or create functions, procedures,
## Packages and any other object that contain PL/SQL code, exmaple
#begin
#   -- any pl/sql code
#end;
#/
################################################################################
## Script could include another file with sql:
#@include.sql
## !!! Avoid include migration scripts
################################################################################
## To continue or break on specific errors use:
#whenever error [pattern] continue|break
################################################################################
## Additional help
## roam-sql -h|--help for command line options
## roam-sql -i|--info for syntax help