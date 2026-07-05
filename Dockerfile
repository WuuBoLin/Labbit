# Copyright (C) 2026 WuuBoLin
# SPDX-License-Identifier: GPL-3.0-or-later

FROM golang:1.26.4-alpine AS build
RUN apk add --no-cache curl libstdc++ libgcc alpine-sdk

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go install github.com/a-h/templ/cmd/templ@latest && \
    templ generate && \
    curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64-musl -o tailwindcss && \
    chmod +x tailwindcss && \
    ./tailwindcss -i cmd/web/styles/input.css -o cmd/web/assets/css/labbit.css

RUN CGO_ENABLED=1 GOOS=linux go build -o labbit cmd/api/main.go

FROM alpine:3.20.1 AS prod
WORKDIR /app
RUN apk add --no-cache ca-certificates libstdc++ libgcc && mkdir -p /data
COPY --from=build /app/labbit /app/labbit
ENV BIND=0.0.0.0
ENV PORT=80
ENV DB_URL=/data/labbit.db
VOLUME ["/data"]
EXPOSE 80
CMD ["./labbit"]
