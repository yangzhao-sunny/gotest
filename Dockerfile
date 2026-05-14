FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /taskmgr ./cmd/server/
RUN go build -o /taskmgr-migrate ./cmd/migrate/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /taskmgr /taskmgr
COPY --from=build /taskmgr-migrate /taskmgr-migrate
COPY migrations/ /migrations/
ENTRYPOINT ["/taskmgr"]
