FROM golang:1.13.6-alpine as builder
WORKDIR /app
COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .
RUN go build -o main ./...

FROM alpine
COPY --from=builder /app/main /
CMD ["/main"]