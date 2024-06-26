FROM golang:1.22-alpine as builder
ENV CGO_ENABLED=0
ENV GOOS=linux

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -ldflags "-s -w -extldflags -static" -o /cmon_sd


# final image
FROM scratch
EXPOSE 8080
COPY --from=builder /cmon_sd /
CMD ["/cmon_sd"]
