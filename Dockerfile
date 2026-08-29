# Сборка серверного слоя dnd. Двухстадийная: тулчейн Go собирает статический
# бинарь, финальный образ — distroless (ни shell, ни пакетного менеджера, ни
# лишней поверхности атаки). Миграции вшиты в бинарь (go:embed); дела копируются
# отдельно — их читает CASES_DIR на старте.

FROM golang:1.26 AS build
WORKDIR /src

# Слой зависимостей кэшируется отдельно: правка кода не тянет повторный download.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO выключен: pgx и весь код — чистый Go, бинарь статический и ложится в
# distroless/static без libc.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /src/cases /app/cases
ENV CASES_DIR=/app/cases
# Railway пробрасывает свой PORT; 8080 — дефолт бинаря и подсказка читателю.
EXPOSE 8080
ENTRYPOINT ["/app/server"]
