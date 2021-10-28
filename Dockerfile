FROM golang:1.16.7-alpine3.14 AS builder

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod verify
RUN go mod download

COPY internal/ internal/
COPY main.go main.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# FROM gcr.io/distroless/static:nonroot
FROM alpine:latest
WORKDIR /
COPY --from=builder /workspace/manager .
USER root

ENTRYPOINT ["/manager"]
