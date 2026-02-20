FROM postgres:18

COPY init_seed_data.sql /docker-entrypoint-initdb.d/init_seed_data.sql
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
