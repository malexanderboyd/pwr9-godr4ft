FROM golang:1.13.6-alpine as builder
WORKDIR /app
COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .
RUN go build -o main github.com/malexanderboyd/pwr9-godr4ft/cmd/godr4ft

FROM alpine
COPY --from=builder /app/main /
CMD ["/main"]