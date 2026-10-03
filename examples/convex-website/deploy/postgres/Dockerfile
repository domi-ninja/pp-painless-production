FROM postgres:16-alpine
COPY entrypoint.sh /usr/local/bin/project-postgres-entrypoint.sh
ENTRYPOINT ["bash", "/usr/local/bin/project-postgres-entrypoint.sh"]
CMD ["postgres"]
