# Build context is the repository root:
#   docker build -f pkg/mcphc/example/Dockerfile -t healthcheck-mcp .
FROM golang:1.26 AS build
WORKDIR /src
COPY . .
WORKDIR /src/pkg/mcphc
RUN CGO_ENABLED=0 GOWORK=off go build -o /hc ./example

FROM gcr.io/distroless/static
COPY --from=build /hc /hc
EXPOSE 8080
ENTRYPOINT ["/hc"]
