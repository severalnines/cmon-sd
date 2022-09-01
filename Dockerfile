FROM golang:1.19-alpine as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -o /cmon_sd


# final image
FROM scratch
EXPOSE 8080
COPY --from=builder /cmon_sd /
CMD ["/cmon_sd"]
