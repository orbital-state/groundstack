# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 go build -o /out/turquoise-api ./cmd/turquoise-api

FROM alpine:3.20

RUN adduser -D -H -u 10001 app
USER app

EXPOSE 8080

COPY --from=build /out/turquoise-api /usr/local/bin/turquoise-api

ENTRYPOINT ["/usr/local/bin/turquoise-api"]
