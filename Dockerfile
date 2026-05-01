FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mcportal ./cmd/mcportal

FROM alpine:3.22

RUN addgroup -S app && adduser -S -G app app

WORKDIR /app

COPY --from=build /out/mcportal /usr/local/bin/mcportal

USER app

ENV MCPORTAL_ADDR=:8080
ENV MCPORTAL_DATA_DIR=/app/data

EXPOSE 8080

CMD ["mcportal"]
