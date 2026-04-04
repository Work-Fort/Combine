FROM alpine:latest

# Create directories
WORKDIR /combine
# Expose data volume
VOLUME /combine

# Environment variables
ENV COMBINE_DATA_PATH "/combine"
ENV COMBINE_INITIAL_ADMIN_KEYS ""
# workaround to prevent slowness in docker when running with a tty
ENV CI "1"

# Expose ports
# SSH
EXPOSE 23231/tcp
# HTTP
EXPOSE 23232/tcp
# Stats
EXPOSE 23233/tcp
# Set the default command
ENTRYPOINT [ "/usr/local/bin/combine", "serve" ]

RUN apk update && apk add --update git bash openssh && rm -rf /var/cache/apk/*

COPY combine /usr/local/bin/combine
