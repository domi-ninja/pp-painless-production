FROM postgres:16.15-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea
COPY entrypoint.sh /usr/local/bin/project-postgres-entrypoint.sh
ENTRYPOINT ["bash", "/usr/local/bin/project-postgres-entrypoint.sh"]
CMD ["postgres"]
