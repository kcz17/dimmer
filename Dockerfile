FROM golang:latest as builder
RUN mkdir /build
ADD . /build/
WORKDIR /build
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags '-w -extldflags "-static"' -o main .
FROM scratch
COPY --from=builder /build/main /app/
WORKDIR /app
CMD ["./main"]
