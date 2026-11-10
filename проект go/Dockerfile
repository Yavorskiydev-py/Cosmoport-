# Один Dockerfile для всех трёх сервисов. Нужный сервис выбирается аргументом SERVICE:
#   docker build --build-arg SERVICE=api .
#   docker build --build-arg SERVICE=mission-control .
#   docker build --build-arg SERVICE=loadgen .

# ---------- этап сборки ----------
FROM golang:1.26-alpine AS build
WORKDIR /src

# Сначала только go.mod/go.sum — слой с зависимостями кэшируется между сборками.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG SERVICE=api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${SERVICE}

# ---------- финальный образ ----------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -u 10001 app
USER app
COPY --from=build /out/app /usr/local/bin/app
ENTRYPOINT ["/usr/local/bin/app"]
