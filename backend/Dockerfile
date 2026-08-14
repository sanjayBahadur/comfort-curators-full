FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# BUILD_TAGS is empty (production) unless explicitly set to "acceptance" by
# the caller. Only scripts/run-phase does that, to compile in the
# acceptance-only session-fixture route (internal/iam/testfixtures.go) that
# the acceptance suite needs and no real deployment should ever have. A
# plain `docker compose up` or `docker build` with no --build-arg leaves
# this empty, so that route's code is not in the binary at all.
ARG BUILD_TAGS=""
RUN CGO_ENABLED=0 go build ${BUILD_TAGS:+-tags "$BUILD_TAGS"} -o /bin/api ./cmd/api/
RUN CGO_ENABLED=0 go build ${BUILD_TAGS:+-tags "$BUILD_TAGS"} -o /bin/worker ./cmd/worker/
RUN CGO_ENABLED=0 go build -o /bin/acceptance-seed ./internal/platform/acceptance/seed/
RUN CGO_ENABLED=0 go build -o /bin/model-stub ./cmd/model-stub/

FROM alpine:3.19 AS api
RUN apk add --no-cache ca-certificates curl tzdata
COPY --from=builder /bin/api /usr/local/bin/api
USER 65534
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=2s --start-period=5s --retries=3 \
	CMD ["curl", "-f", "http://localhost:8080/health/live"]
ENTRYPOINT ["api"]

FROM alpine:3.19 AS worker
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/worker /usr/local/bin/worker
USER 65534
ENTRYPOINT ["worker"]

FROM alpine:3.19 AS seed
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/acceptance-seed /usr/local/bin/acceptance-seed
ENTRYPOINT ["acceptance-seed"]

FROM alpine:3.19 AS model-stub
RUN apk add --no-cache ca-certificates curl tzdata
COPY --from=builder /bin/model-stub /usr/local/bin/model-stub
USER 65534
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=2s --start-period=5s --retries=3 \
	CMD ["curl", "-f", "http://localhost:8080/health/live"]
ENTRYPOINT ["model-stub"]
