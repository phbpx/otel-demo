FROM golang:1.18-alpine as builder
RUN mkdir /build
ADD . /build/
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o servid ./cmd/servid/main.go

FROM alpine:3.15.1
COPY --from=builder /build/servid /home/appuser/servid

# Create a group and user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN chmod +x /home/appuser/servid

USER appuser
CMD ["/home/appuser/servid"]