ARG BASE_IMAGE=registry.opencode.de/bmi/opendesk/components/platform-development/images/helm:1.5.0

FROM golang:1.26.0 AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/kubesleep ./cmd/kubesleep

FROM ${BASE_IMAGE}

ARG TARGETPLATFORM
COPY --from=build /usr/local/bin/kubesleep /kubesleep
USER nonroot:nonroot
ENTRYPOINT ["/kubesleep"]
