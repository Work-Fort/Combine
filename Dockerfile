FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG GIT_SHA=unknown
ARG GIT_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w \
        -X github.com/Work-Fort/Combine/cmd.Version=${VERSION} \
        -X github.com/Work-Fort/Combine/cmd.CommitSHA=${GIT_SHA} \
        -X github.com/Work-Fort/Combine/cmd.CommitDate=${GIT_DATE}" \
      -o /combine ./cmd/combine

FROM alpine:3.21
RUN apk add --no-cache ca-certificates git
COPY --from=build /combine /usr/local/bin/combine
VOLUME /combine-data
ENV COMBINE_DATA_PATH="/combine-data"
EXPOSE 23231/tcp 23232/tcp 23233/tcp
ENTRYPOINT ["combine"]
CMD ["serve"]
